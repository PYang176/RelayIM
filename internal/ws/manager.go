package ws

import (
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"sync"
)

// ConnManager 在线连接管理器
type ConnManager struct {
	// key: userID, value: *websocket.Conn
	connections sync.Map
	mutex       sync.RWMutex
}

// Manager 全局连接管理器
var Manager = &ConnManager{}

// Add 添加用户连接
func (m *ConnManager) Add(userID uint, conn *websocket.Conn) {
	// 加锁保证原子操作
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 如果已存在旧连接，关闭旧连接
	if oldConn, ok := m.connections.Load(userID); ok {
		err := oldConn.(*websocket.Conn).Close()
		if err != nil {
			fmt.Println("旧连接关闭失败", err)
			return
		}
	}

	// 添加新连接
	m.connections.Store(userID, conn)
	log.Printf("用户 %d 已上线\n", userID)
}

// Remove 移除用户连接
func (m *ConnManager) Remove(userID uint) {
	// 加锁保证原子操作
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 如果连接存在则关闭
	if conn, ok := m.connections.Load(userID); ok {
		err := conn.(*websocket.Conn).Close()
		if err != nil {
			fmt.Println("连接关闭失败", err)
			return
		}
		log.Printf("用户 %d 已下线\n", userID)
	}

}

// Send 给指定用户发送消息
func (m *ConnManager) Send(userID uint, message []byte) bool {
	if conn, ok := m.connections.Load(userID); ok {
		err := conn.(*websocket.Conn).WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf("发送消息 %d 失败，错误为%v\n", userID, err)
			//发送消息失败，移除无效连接
			m.Remove(userID)
			return false
		}
		return true
	}
	return false

}

// Broadcast 广播方法
func (m *ConnManager) Broadcast(message []byte) {
	// 遍历所有键值对，依次进行发消息广播
	m.connections.Range(func(key, value any) bool {
		conn := value.(*websocket.Conn)
		err := conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf(" 广播失败 userID=%v, err=%v", key, err)
			// 广播失败则清除死连接
			m.Remove(key.(uint))

		}
		return true
	})
}

// IsOnline 判断是否在线
func (m *ConnManager) IsOnline(userID uint) bool {
	_, ok := m.connections.Load(userID)
	return ok
}

// Count 获取在线人数
func (m *ConnManager) Count() int {
	count := 0
	// 遍历键值对统计在线人数
	m.connections.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
