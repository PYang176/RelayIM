// Package app 负责组装应用依赖并管理服务生命周期。
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ginChatProject/internal/consumer"
	"ginChatProject/internal/platform/bootstrap"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/router"
	"ginChatProject/internal/task"
)

const (
	defaultServerAddr = ":8081"
	shutdownTimeout   = 10 * time.Second
)

// Run 初始化依赖、启动后台任务并持续提供 HTTP 服务，
// 直到进程收到中断或终止信号。
func Run() error {
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
	if err := bootstrap.InitSnowFlake(); err != nil {
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
	defer sqlDB.Close()

	// 启动阶段 2：启动后台任务。Consumer 在 Run 返回时停止接收新的任务。
	userRepo := repository.NewUserRepository(db)
	heartbeatTask := task.NewHeartbeatSyncTask(userRepo)
	go heartbeatTask.Start()

	msgRepo := repository.NewMessageRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	msgConsumer := consumer.NewGroupMsgConsumer(msgRepo, groupRepo)
	msgConsumer.Start()
	defer msgConsumer.Stop()

	// 启动阶段 3：把已组装的依赖交给 Router。这里不读取环境变量或数据库，
	// 只负责 HTTP 监听和连接池生命周期。
	server := &http.Server{
		Addr:              serverAddr(),
		Handler:           router.Router(db),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return runServer(server)
}

func serverAddr() string {
	if addr := strings.TrimSpace(os.Getenv("GINCHAT_SERVER_ADDR")); addr != "" {
		return addr
	}
	return defaultServerAddr
}

func runServer(server *http.Server) error {
	// 信号 Context 只负责监听生命周期；每个后台任务仍应拥有自己的取消机制。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 缓冲 1 个错误，确保 ListenAndServe 退出时不会因无人接收而泄漏 Goroutine。
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP 服务监听于 %s", server.Addr)
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
		log.Println("HTTP 服务已停止")
		return nil
	}
}
