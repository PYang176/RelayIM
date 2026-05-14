package online

// OnlineChecker 查询是否在线
type OnlineChecker interface {
	IsOnline(userID uint) bool
}
