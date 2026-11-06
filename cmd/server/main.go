package main

import (
	"log"

	"ginChatProject/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
