// Package app assembles the application dependencies and manages its lifecycle.
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

	"ginChatProject/internal/config"
	"ginChatProject/internal/consumer"
	"ginChatProject/internal/platform/bootstrap"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/router"
	"ginChatProject/internal/task"
	"github.com/spf13/viper"
)

const (
	defaultServerAddr = ":8081"
	shutdownTimeout   = 10 * time.Second
)

// Run initializes dependencies, starts background workers and serves HTTP until
// the process receives an interrupt or termination signal.
func Run() error {
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

	var mysqlCfg config.MySQLConfig
	if err := viper.UnmarshalKey("mysql", &mysqlCfg); err != nil {
		return fmt.Errorf("解析 MySQL 配置失败: %w", err)
	}

	db, err := bootstrap.InitMySQL(mysqlCfg)
	if err != nil {
		return fmt.Errorf("初始化 MySQL 失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取 MySQL 连接池失败: %w", err)
	}
	defer sqlDB.Close()

	userRepo := repository.NewUserRepository(db)
	heartbeatTask := task.NewHeartbeatSyncTask(userRepo)
	go heartbeatTask.Start()

	msgRepo := repository.NewMessageRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	msgConsumer := consumer.NewGroupMsgConsumer(msgRepo, groupRepo)
	msgConsumer.Start()
	defer msgConsumer.Stop()

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 HTTP 服务失败: %w", err)
		}
		log.Println("HTTP 服务已停止")
		return nil
	}
}
