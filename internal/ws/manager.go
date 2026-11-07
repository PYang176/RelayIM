package ws

import (
	"sync"
)

const maxConnectionsPerUser = 10

type managedConnection interface {
	UserID() uint64
	Enqueue(message []byte) bool
	Close() error
}

// ConnManager 管理同一进程内的 WebSocket 连接。
// 每个用户可以保留多个连接，移除时必须按具体连接执行，不能按用户整体删除。
type ConnManager struct {
	mutex sync.RWMutex
	users map[uint64]map[managedConnection]struct{}
}

// Manager 是当前单实例运行时的默认连接管理器。
var Manager = NewConnManager()

// NewConnManager 创建空连接管理器。
func NewConnManager() *ConnManager {
	return &ConnManager{
		users: make(map[uint64]map[managedConnection]struct{}),
	}
}

// Add 注册一个连接，返回 false 表示该用户连接数已达到上限。
func (m *ConnManager) Add(connection managedConnection) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	userID := connection.UserID()
	connections := m.users[userID]
	if connections == nil {
		connections = make(map[managedConnection]struct{})
		m.users[userID] = connections
	}
	if _, exists := connections[connection]; exists {
		return true
	}
	if len(connections) >= maxConnectionsPerUser {
		return false
	}
	connections[connection] = struct{}{}
	return true
}

// Remove 只移除指定连接，不会误删同一用户的新连接或其他设备连接。
func (m *ConnManager) Remove(connection managedConnection) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	userID := connection.UserID()
	connections := m.users[userID]
	if connections == nil {
		return
	}
	delete(connections, connection)
	if len(connections) == 0 {
		delete(m.users, userID)
	}
}

// Send 向指定用户的全部在线连接投递消息；返回是否至少入队成功一次。
func (m *ConnManager) Send(userID uint, message []byte) bool {
	return m.sendUser(uint64(userID), message) > 0
}

// SendToUsers 向多个用户的全部在线连接投递消息，返回成功入队的连接数。
func (m *ConnManager) SendToUsers(userIDs []uint64, message []byte) int {
	if len(userIDs) == 0 || len(message) == 0 {
		return 0
	}

	uniqueUsers := make(map[uint64]struct{}, len(userIDs))
	delivered := 0
	for _, userID := range userIDs {
		if _, exists := uniqueUsers[userID]; exists {
			continue
		}
		uniqueUsers[userID] = struct{}{}
		delivered += m.sendUser(userID, message)
	}
	return delivered
}

func (m *ConnManager) sendUser(userID uint64, message []byte) int {
	connections := m.snapshotUser(userID)
	delivered := 0
	for _, connection := range connections {
		if connection.Enqueue(message) {
			delivered++
			continue
		}
		// 慢连接或已关闭连接只移除自身，不影响用户的其他设备。
		m.Remove(connection)
	}
	return delivered
}

func (m *ConnManager) snapshotUser(userID uint64) []managedConnection {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	connections := m.users[userID]
	result := make([]managedConnection, 0, len(connections))
	for connection := range connections {
		result = append(result, connection)
	}
	return result
}

// Broadcast 向当前进程内所有连接投递消息。
func (m *ConnManager) Broadcast(message []byte) int {
	m.mutex.RLock()
	connections := make([]managedConnection, 0)
	for _, userConnections := range m.users {
		for connection := range userConnections {
			connections = append(connections, connection)
		}
	}
	m.mutex.RUnlock()

	delivered := 0
	for _, connection := range connections {
		if connection.Enqueue(message) {
			delivered++
			continue
		}
		m.Remove(connection)
	}
	return delivered
}

// IsOnline 判断用户是否至少有一个有效连接。
func (m *ConnManager) IsOnline(userID uint) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.users[uint64(userID)]) > 0
}

// Count 返回当前在线用户数，不把同一用户的多台设备重复计算。
func (m *ConnManager) Count() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.users)
}

// ConnectionCount 返回当前进程内的连接数量。
func (m *ConnManager) ConnectionCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	count := 0
	for _, connections := range m.users {
		count += len(connections)
	}
	return count
}

// CloseAll 关闭全部连接，供测试或进程级清理使用。
func (m *ConnManager) CloseAll() {
	m.mutex.RLock()
	connections := make([]managedConnection, 0)
	for _, userConnections := range m.users {
		for connection := range userConnections {
			connections = append(connections, connection)
		}
	}
	m.mutex.RUnlock()

	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			continue
		}
	}
}
