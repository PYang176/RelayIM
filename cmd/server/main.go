package main

import (
	"log/slog"
	"os"

	"relayim/internal/app"
)

// main 启动 RelayIM 服务并处理启动阶段错误
func main() {
	if err := app.Run(); err != nil {
		slog.Error("服务异常退出", "error", err)
		os.Exit(1)
	}
}
