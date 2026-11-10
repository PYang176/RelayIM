package ws

import (
	"sync"
	"testing"
)

type fakeConnection struct {
	userID   uint64
	mutex    sync.Mutex
	messages [][]byte
	full     bool
	closed   bool
}

// UserID 返回测试连接所属用户 ID
func (c *fakeConnection) UserID() uint64 {
	return c.userID
}

// Enqueue 把测试消息写入连接队列，模拟 WebSocket 有界发送行为
func (c *fakeConnection) Enqueue(message []byte) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.closed || c.full {
		return false
	}
	c.messages = append(c.messages, append([]byte(nil), message...))
	return true
}

// Close 关闭并释放底层资源
func (c *fakeConnection) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.closed = true
	return nil
}

// messageCount 返回测试连接中已入队的消息数量
func (c *fakeConnection) messageCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.messages)
}

// TestConnManagerSupportsMultipleDevicesAndExactRemoval 覆盖多设备连接与按连接精确移除
func TestConnManagerSupportsMultipleDevicesAndExactRemoval(t *testing.T) {
	manager := NewConnManager()
	first := &fakeConnection{userID: 42}
	second := &fakeConnection{userID: 42}
	otherUser := &fakeConnection{userID: 99}

	if !manager.Add(first) || !manager.Add(second) || !manager.Add(otherUser) {
		t.Fatal("注册连接失败")
	}
	if !manager.Send(42, []byte("hello")) {
		t.Fatal("向在线用户发送失败")
	}
	if first.messageCount() != 1 || second.messageCount() != 1 {
		t.Fatalf("设备消息数 = %d/%d，期望各 1",
			first.messageCount(),
			second.messageCount(),
		)
	}

	// 移除第一台设备不能误删同一用户的第二台设备。
	manager.Remove(first)
	if !manager.IsOnline(42) {
		t.Fatal("移除一个连接后用户不应下线")
	}
	if first.messageCount() != 1 || second.messageCount() != 1 {
		t.Fatalf("移除前消息数异常: %d/%d",
			first.messageCount(),
			second.messageCount(),
		)
	}

	manager.Send(42, []byte("again"))
	if first.messageCount() != 1 {
		t.Fatalf("已移除连接收到新消息: %d", first.messageCount())
	}
	if second.messageCount() != 2 {
		t.Fatalf("保留连接消息数 = %d，期望 2", second.messageCount())
	}
}

// TestConnManagerCountsUsersAndConnections 覆盖连接管理器对用户数和连接数的统计
func TestConnManagerCountsUsersAndConnections(t *testing.T) {
	manager := NewConnManager()
	manager.Add(&fakeConnection{userID: 1})
	manager.Add(&fakeConnection{userID: 1})
	manager.Add(&fakeConnection{userID: 2})

	if got := manager.Count(); got != 2 {
		t.Fatalf("在线用户数 = %d，期望 2", got)
	}
	if got := manager.ConnectionCount(); got != 3 {
		t.Fatalf("在线连接数 = %d，期望 3", got)
	}
}

// TestConnManagerSendToUsersDeduplicatesAndRemovesFullConnection 覆盖批量发送去重和已满连接移除
func TestConnManagerSendToUsersDeduplicatesAndRemovesFullConnection(t *testing.T) {
	manager := NewConnManager()
	healthy := &fakeConnection{userID: 1}
	full := &fakeConnection{userID: 2, full: true}
	manager.Add(healthy)
	manager.Add(full)

	delivered := manager.SendToUsers(
		[]uint64{1, 1, 2},
		[]byte("message"),
	)
	if delivered != 1 {
		t.Fatalf("成功投递连接数 = %d，期望 1", delivered)
	}
	if healthy.messageCount() != 1 {
		t.Fatalf("健康连接消息数 = %d，期望 1", healthy.messageCount())
	}
	if manager.IsOnline(2) {
		t.Fatal("队列已满的连接应从管理器移除")
	}
}

// TestConnManagerRejectsExcessiveConnectionsPerUser 覆盖单用户连接数达到上限后的拒绝行为
func TestConnManagerRejectsExcessiveConnectionsPerUser(t *testing.T) {
	manager := NewConnManager()
	for index := 0; index < maxConnectionsPerUser; index++ {
		if !manager.Add(&fakeConnection{userID: 1}) {
			t.Fatalf("第 %d 个连接注册失败", index+1)
		}
	}
	if manager.Add(&fakeConnection{userID: 1}) {
		t.Fatal("超过单用户连接上限后仍注册成功")
	}
}
