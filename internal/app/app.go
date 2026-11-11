// Package app 负责组装应用依赖并管理服务生命周期。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PYang176/RelayIM/internal/messaging"
	"github.com/PYang176/RelayIM/internal/platform/bootstrap"
	"github.com/PYang176/RelayIM/internal/platform/objectstore"
	"github.com/PYang176/RelayIM/internal/platform/observability"
	"github.com/PYang176/RelayIM/internal/platform/ratelimit"
	"github.com/PYang176/RelayIM/internal/platform/redisstore"
	"github.com/PYang176/RelayIM/internal/presence"
	"github.com/PYang176/RelayIM/internal/realtime"
	"github.com/PYang176/RelayIM/internal/router"
	"github.com/PYang176/RelayIM/internal/ws"
	"gorm.io/gorm"
)

const (
	defaultServerAddr = ":8081"
	shutdownTimeout   = 10 * time.Second
)

// Run 初始化依赖、启动后台任务并持续提供 HTTP 服务，
// 直到进程收到中断或终止信号。
func Run() error {
	setupLogging()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// 启动阶段 1：加载配置并初始化有状态基础设施。任一依赖失败都立即返回，
	// 避免服务带着半套依赖继续运行。
	if err := bootstrap.InitConfig(); err != nil {
		return err
	}
	if err := bootstrap.InitJWT(); err != nil {
		return err
	}
	if err := bootstrap.InitRedis(); err != nil {
		return err
	}
	mysqlCfg := bootstrap.LoadMySQLConfig()
	db, err := bootstrap.InitMySQL(mysqlCfg)
	if err != nil {
		return fmt.Errorf("初始化 MySQL 失败: %w", err)
	}

	// HTTP 服务退出时必须释放数据库连接池。
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取 MySQL 连接池失败: %w", err)
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			slog.Warn("关闭 MySQL 连接池失败", "error", closeErr)
		}
	}()
	if err := bootstrap.InitSnowFlake(ctx, db); err != nil {
		return err
	}

	metrics := observability.NewMetrics()
	metrics.RegisterDatabaseStats(sqlDB.Stats)
	metrics.RegisterWebSocketConnections(ws.Manager.ConnectionCount)
	metrics.RegisterOutboxStats(func(ctx context.Context) (observability.OutboxStats, error) {
		return loadOutboxStats(ctx, db)
	})
	health := observability.NewHealth(
		observability.DependencyCheck{
			Name:  "mysql",
			Check: sqlDB.PingContext,
		},
		observability.DependencyCheck{
			Name: "redis",
			Check: func(ctx context.Context) error {
				return redisstore.RDB.Ping(ctx).Err()
			},
		},
	)
	rateLimitConfig := bootstrap.LoadWSTicketRateLimitConfig()
	slidingWindowLimiter := ratelimit.NewSlidingWindowLimiter(
		redisstore.RDB,
		"relayim",
	)
	wsTicketLimiter := ratelimit.NewWSTicketLimiter(
		slidingWindowLimiter,
		rateLimitConfig,
	)
	serviceInstanceID := instanceID()
	slog.Info("服务实例标识已生成", "instance_id", serviceInstanceID)
	shutdownTracing, err := observability.InitTracing(
		ctx,
		bootstrap.LoadTracingConfig(),
		serviceInstanceID,
	)
	if err != nil {
		return fmt.Errorf("初始化 OpenTelemetry Trace: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			5*time.Second,
		)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			slog.Warn("关闭 OpenTelemetry Trace 失败", "error", err)
		}
	}()
	presenceService := presence.NewService(redisstore.RDB, "relayim")
	realtimeRegistry := realtime.NewRegistry(
		redisstore.RDB,
		"relayim",
		serviceInstanceID,
	)

	storageCfg := bootstrap.LoadObjectStorageConfig()
	var attachmentStorage messaging.AttachmentStorage
	if storageCfg.AccessKey == "" || storageCfg.SecretKey == "" {
		slog.Warn("未配置 S3 凭据，附件上传保持禁用")
	} else {
		client, err := objectstore.NewClient(ctx, storageCfg)
		if err != nil {
			return fmt.Errorf("初始化附件存储失败: %w", err)
		}
		attachmentStorage = client
	}

	// 启动阶段 2：启动具备明确取消语义的 RabbitMQ 后台任务。
	var backgroundWorkers sync.WaitGroup
	var deliveryRouter *realtime.DeliveryRouter
	messagingRepository := messaging.NewRepository(db)
	defer func() {
		// Run 退出时先取消后台任务，再等待连接和轮询协程释放资源。
		stop()
		backgroundWorkers.Wait()
		if deliveryRouter != nil {
			if err := deliveryRouter.Close(); err != nil {
				slog.Warn("关闭跨实例实时投递发布器失败", "error", err)
			}
		}
	}()

	cleanupCfg := bootstrap.LoadAttachmentCleanupConfig()
	if cleanupCfg.Enabled && attachmentStorage != nil {
		pendingTTL := time.Duration(cleanupCfg.PendingTTLHours) * time.Hour
		claimLease := time.Duration(cleanupCfg.ClaimLeaseSeconds) * time.Second
		metrics.RegisterAttachmentOrphanStats(
			func(loadCtx context.Context) (int64, error) {
				return messagingRepository.CountOrphanAttachments(
					loadCtx,
					time.Now().UTC(),
					pendingTTL,
					claimLease,
				)
			},
		)
		cleanupWorker := messaging.NewAttachmentCleanupWorker(
			messagingRepository,
			attachmentStorage,
			cleanupCfg,
			metrics,
		)
		backgroundWorkers.Add(1)
		go func() {
			defer backgroundWorkers.Done()
			cleanupWorker.Run(ctx)
		}()
	}

	rabbitCfg := bootstrap.LoadRabbitMQConfig()
	if rabbitCfg.URL != "" {
		outboxWorker := messaging.NewOutboxWorker(
			messagingRepository,
			rabbitCfg,
			serviceInstanceID,
		)
		backgroundWorkers.Add(1)
		go func() {
			defer backgroundWorkers.Done()
			outboxWorker.Run(ctx)
		}()

		deliveryRouter = realtime.NewDeliveryRouter(
			realtimeRegistry,
			rabbitCfg,
			serviceInstanceID,
		)
		eventConsumer := messaging.NewEventConsumer(
			messaging.NewEventProcessor(db, deliveryRouter),
			rabbitCfg,
			serviceInstanceID+"-message-events",
			metrics,
		)
		backgroundWorkers.Add(1)
		go func() {
			defer backgroundWorkers.Done()
			eventConsumer.Run(ctx)
		}()

		nodeConsumer := realtime.NewNodeConsumer(
			rabbitCfg,
			serviceInstanceID,
			ws.Manager,
		)
		backgroundWorkers.Add(1)
		go func() {
			defer backgroundWorkers.Done()
			nodeConsumer.Run(ctx)
		}()
	} else {
		slog.Warn("未配置 RabbitMQ 地址，后台任务保持禁用")
	}

	// 启动阶段 3：把已组装的依赖交给 Router。这里不读取环境变量或数据库，
	// 只负责 HTTP 监听和连接池生命周期。
	server := &http.Server{
		Addr: serverAddr(),
		Handler: router.Router(
			db,
			attachmentStorage,
			metrics,
			health,
			slidingWindowLimiter,
			wsTicketLimiter,
			presenceService,
			realtimeRegistry,
			deliveryRouter,
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return runServer(ctx, server)
}

// setupLogging 配置Logging运行时依赖
func setupLogging() {
	handler := slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo},
	)
	slog.SetDefault(slog.New(handler))
}

type outboxStatusCount struct {
	Status int   `gorm:"column:status"`
	Count  int64 `gorm:"column:count"`
}

// loadOutboxStats 加载Outbox统计所需数据
func loadOutboxStats(
	ctx context.Context,
	db *gorm.DB,
) (observability.OutboxStats, error) {
	var records []outboxStatusCount
	if err := db.WithContext(ctx).
		Table("outbox_events").
		Select("status, COUNT(*) AS count").
		Group("status").
		Scan(&records).Error; err != nil {
		return observability.OutboxStats{}, fmt.Errorf("查询 Outbox 指标: %w", err)
	}
	var result observability.OutboxStats
	for _, record := range records {
		switch record.Status {
		case 0:
			result.Pending = record.Count
		case 1:
			result.Processing = record.Count
		case 2:
			result.Published = record.Count
		case 3:
			result.Dead = record.Count
		}
	}
	return result, nil
}

// serverAddr 读取服务监听地址配置，缺省时返回默认端口
func serverAddr() string {
	if addr := strings.TrimSpace(os.Getenv("RELAYIM_SERVER_ADDR")); addr != "" {
		return addr
	}
	return defaultServerAddr
}

// runServer 运行服务端直到上下文结束
func runServer(ctx context.Context, server *http.Server) error {
	// 缓冲 1 个错误，确保 ListenAndServe 退出时不会因无人接收而泄漏 Goroutine。
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("HTTP 服务开始监听", "addr", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP 服务异常: %w", err)
	case <-ctx.Done():
		// 给正在处理的请求最多 10 秒完成，避免收到信号后立即粗暴退出。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 HTTP 服务失败: %w", err)
		}
		slog.Info("HTTP 服务已停止")
		return nil
	}
}

// instanceID 生成包含主机、进程和启动时间的实例标识
func instanceID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "relayim"
	}
	// PID 可能被操作系统复用。加入进程启动时间，保证临时节点队列和 Redis
	// 路由不会与已经退出的旧进程混用。
	return fmt.Sprintf(
		"%s-%d-%d",
		hostname,
		os.Getpid(),
		time.Now().UnixNano(),
	)
}
