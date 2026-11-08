package main

import (
	"log/slog"
	"os"

	"ginChatProject/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		slog.Error("服务异常退出", "error", err)
		os.Exit(1)
	}
}
