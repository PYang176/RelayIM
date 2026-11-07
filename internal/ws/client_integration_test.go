package ws

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClientConcurrentProducersUseSingleWriter(t *testing.T) {
	manager := NewConnManager()
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		client := NewClient(7, connection, nil)
		if !manager.Add(client) {
			_ = connection.Close()
			return
		}
		defer manager.Remove(client)
		client.Run(request.Context(), nil)
	}))
	defer server.Close()

	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http")
	connection, _, err := websocket.DefaultDialer.Dial(websocketURL, nil)
	if err != nil {
		t.Fatalf("建立测试 WebSocket 失败: %v", err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("设置读取超时失败: %v", err)
	}

	waitFor(t, func() bool { return manager.IsOnline(7) })

	const messageCount = 20
	results := make(chan bool, messageCount)
	var waitGroup sync.WaitGroup
	for index := 0; index < messageCount; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			results <- manager.Send(7, []byte(fmt.Sprintf("message-%d", index)))
		}(index)
	}
	waitGroup.Wait()
	close(results)
	for delivered := range results {
		if !delivered {
			t.Fatal("并发发送期间有消息未入队")
		}
	}

	received := make(map[string]bool, messageCount)
	for len(received) < messageCount {
		_, payload, err := connection.ReadMessage()
		if err != nil {
			t.Fatalf("读取 WebSocket 消息失败: %v", err)
		}
		received[string(payload)] = true
	}
	if len(received) != messageCount {
		t.Fatalf("收到唯一消息数 = %d，期望 %d", len(received), messageCount)
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("关闭客户端连接失败: %v", err)
	}
	waitFor(t, func() bool { return !manager.IsOnline(7) })
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待条件满足超时")
}
