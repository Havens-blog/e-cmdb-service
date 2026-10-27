package domain

import "time"

// API Token 状态枚举。
const (
	APITokenStatusActive  = "active"
	APITokenStatusRevoked = "revoked"
)

// ApiToken API Token 领域模型。
//
// TokenHash 仅保存 SHA-256(hex(token)) 的 hex，绝不保存明文；明文只在创建
// 接口的响应里返回一次。ExpireAt 为毫秒时间戳，0 表示永不过期。
type ApiToken struct {
	ID         int64
	TenantID   int64
	Name       string
	Scopes     []string
	Status     string
	TokenHash  string
	ExpireAt   int64
	CreateTime time.Time
	UpdateTime time.Time
}
