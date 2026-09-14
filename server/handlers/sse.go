package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
)

// ValidateTokenAndGetUserID 验证token并获取用户ID
func ValidateTokenAndGetUserID(tokenString string) (uint, error) {
	// 验证token
	claims, err := verifyToken(tokenString)
	if err != nil {
		return 0, err
	}

	// 检查用户是否存在且激活
	var user models.Employee
	if err := models.DB.First(&user, claims.UserID).Error; err != nil {
		return 0, err
	}

	if !user.IsActive {
		return 0, fmt.Errorf("用户账户已被禁用")
	}

	return claims.UserID, nil
}

// SSE消息结构
type SSEMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp string      `json:"timestamp"`
	ID        string      `json:"id"`
}

// SSE事件数据结构
type SSEEventData struct {
	ID           uint        `json:"id"`
	EmployeeID   uint        `json:"employee_id"`
	OperatorID   uint        `json:"operator_id"`
	OperatorName string      `json:"operator_name"`
	Message      string      `json:"message"`
	Timestamp    string      `json:"timestamp"`
	Payload      interface{} `json:"payload,omitempty"`
}

// SSE连接结构
type SSEConnection struct {
	UserID       uint
	ConnectionID string
	Channel      chan SSEMessage
	Context      context.Context
	CancelFunc   context.CancelFunc
	LastPing     time.Time
}

// SSE连接管理器
type SSEManager struct {
	connections map[string]*SSEConnection // key: connectionID
	userConns   map[uint][]string         // key: userID, value: connectionIDs
	mutex       sync.RWMutex
}

// 全局SSE管理器
var sseManager = &SSEManager{
	connections: make(map[string]*SSEConnection),
	userConns:   make(map[uint][]string),
}

// 添加连接
func (m *SSEManager) AddConnection(userID uint, connectionID string, conn *SSEConnection) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 添加连接
	m.connections[connectionID] = conn

	// 添加用户连接映射
	if _, exists := m.userConns[userID]; !exists {
		m.userConns[userID] = []string{}
	}
	m.userConns[userID] = append(m.userConns[userID], connectionID)

	fmt.Printf("用户 %d 创建新连接 %s，当前连接数: %d\n", userID, connectionID, len(m.userConns[userID]))
}

// 移除连接
func (m *SSEManager) RemoveConnection(connectionID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if conn, exists := m.connections[connectionID]; exists {
		userID := conn.UserID
		conn.CancelFunc()
		close(conn.Channel)
		delete(m.connections, connectionID)

		// 从用户连接映射中移除
		if connIDs, exists := m.userConns[userID]; exists {
			for i, id := range connIDs {
				if id == connectionID {
					m.userConns[userID] = append(connIDs[:i], connIDs[i+1:]...)
					break
				}
			}
			// 如果用户没有连接了，删除映射
			if len(m.userConns[userID]) == 0 {
				delete(m.userConns, userID)
			}
		}

		fmt.Printf("用户 %d 连接 %s 已断开\n", userID, connectionID)
	}
}

// 发送消息给指定用户的所有连接
func (m *SSEManager) SendToUser(userID uint, message SSEMessage) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if connIDs, exists := m.userConns[userID]; exists {
		for _, connectionID := range connIDs {
			if conn, exists := m.connections[connectionID]; exists {
				select {
				case conn.Channel <- message:
					fmt.Printf("发送SSE消息给用户 %d 连接 %s: %s\n", userID, connectionID, message.Type)
				case <-time.After(5 * time.Second):
					fmt.Printf("发送SSE消息给用户 %d 连接 %s 超时\n", userID, connectionID)
				}
			}
		}
	}
}

// 发送消息给多个用户
func (m *SSEManager) SendToUsers(userIDs []uint, message SSEMessage) {
	for _, userID := range userIDs {
		m.SendToUser(userID, message)
	}
}

// 获取在线用户数量
func (m *SSEManager) GetOnlineCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.userConns)
}

// 获取总连接数
func (m *SSEManager) GetTotalConnections() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.connections)
}

// 检查用户是否在线
func (m *SSEManager) IsUserOnline(userID uint) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	connIDs, exists := m.userConns[userID]
	return exists && len(connIDs) > 0
}

// 获取用户连接数
func (m *SSEManager) GetUserConnectionCount(userID uint) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if connIDs, exists := m.userConns[userID]; exists {
		return len(connIDs)
	}
	return 0
}

// 清理过期连接
func (m *SSEManager) CleanupExpiredConnections() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	expiredConnections := []string{}

	for connectionID, conn := range m.connections {
		if now.Sub(conn.LastPing) > 5*time.Minute {
			fmt.Printf("清理过期连接：用户 %d 连接 %s\n", conn.UserID, connectionID)
			conn.CancelFunc()
			close(conn.Channel)
			expiredConnections = append(expiredConnections, connectionID)
		}
	}

	// 批量删除过期连接
	for _, connectionID := range expiredConnections {
		if conn, exists := m.connections[connectionID]; exists {
			userID := conn.UserID
			delete(m.connections, connectionID)

			// 从用户连接映射中移除
			if connIDs, exists := m.userConns[userID]; exists {
				for i, id := range connIDs {
					if id == connectionID {
						m.userConns[userID] = append(connIDs[:i], connIDs[i+1:]...)
						break
					}
				}
				// 如果用户没有连接了，删除映射
				if len(m.userConns[userID]) == 0 {
					delete(m.userConns, userID)
				}
			}
		}
	}
}

// SSE事件流处理器
func SSEHandler(c *gin.Context) {
	// 从URL参数获取token并验证
	token := c.Query("token")
	if token == "" {
		c.String(http.StatusUnauthorized, "event: error\ndata: {\"error\": \"未提供认证令牌\"}\n\n")
		return
	}

	// 验证token并获取用户ID
	userID, err := ValidateTokenAndGetUserID(token)
	if err != nil {
		c.String(http.StatusUnauthorized, "event: error\ndata: {\"error\": \"认证失败\"}\n\n")
		return
	}

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 创建连接上下文
	ctx, cancel := context.WithCancel(c.Request.Context())

	// 创建SSE连接
	conn := &SSEConnection{
		UserID:       userID,
		ConnectionID: fmt.Sprintf("conn-%d", time.Now().UnixNano()), // 生成唯一的连接ID
		Channel:      make(chan SSEMessage, 100),
		Context:      ctx,
		CancelFunc:   cancel,
		LastPing:     time.Now(),
	}

	// 添加到管理器
	sseManager.AddConnection(userID, conn.ConnectionID, conn)

	// 连接清理
	defer func() {
		sseManager.RemoveConnection(conn.ConnectionID)
	}()

	// 发送初始连接确认消息
	initialMessage := SSEMessage{
		Type: "connected",
		Data: map[string]interface{}{
			"user_id":   userID,
			"timestamp": time.Now().Format(time.RFC3339),
		},
		Timestamp: time.Now().Format(time.RFC3339),
		ID:        fmt.Sprintf("init-%d", time.Now().UnixNano()),
	}

	c.SSEvent("message", initialMessage)
	c.Writer.Flush()

	// 启动心跳检测
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			// 发送心跳
			heartbeatMessage := SSEMessage{
				Type: "heartbeat",
				Data: map[string]interface{}{
					"timestamp": time.Now().Format(time.RFC3339),
				},
				Timestamp: time.Now().Format(time.RFC3339),
				ID:        fmt.Sprintf("heartbeat-%d", time.Now().UnixNano()),
			}

			c.SSEvent("message", heartbeatMessage)
			c.Writer.Flush()

			// 更新最后ping时间
			conn.LastPing = time.Now()

		case message, ok := <-conn.Channel:
			if !ok {
				return
			}

			c.SSEvent("message", message)
			c.Writer.Flush()
		}
	}
}

// 获取SSE连接状态
func GetSSEStatus(c *gin.Context) {
	// 获取当前用户ID
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未找到用户信息"})
		return
	}
	userID := currentUserID.(uint)

	// 获取连接状态
	isOnline := sseManager.IsUserOnline(userID)
	userConnectionCount := sseManager.GetUserConnectionCount(userID)
	totalConnections := sseManager.GetTotalConnections()
	onlineUsers := sseManager.GetOnlineCount()

	c.JSON(http.StatusOK, gin.H{
		"user_id":               userID,              // 用户ID
		"is_online":             isOnline,            // 是否在线
		"user_connection_count": userConnectionCount, // 用户连接数
		"online_users":          onlineUsers,         // 在线用户数
		"total_connections":     totalConnections,    // 总连接数
	})
}

// 定期清理过期连接的后台任务
func StartSSECleanupTask() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			sseManager.CleanupExpiredConnections()
		}
	}()
}
