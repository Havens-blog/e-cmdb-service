package domain

import "time"

// Webhook 状态枚举。
const (
	WebhookStatusActive   = "active"
	WebhookStatusDisabled = "disabled"
)

// Webhook Webhook 订阅领域模型。
//
// Secret 存储原文，供将来 HMAC 签名使用；列表接口不返回该字段。
type Webhook struct {
	ID         int64
	TenantID   int64
	Name       string
	EventTypes []string
	TargetURL  string
	Secret     string
	Status     string
	CreateTime time.Time
	UpdateTime time.Time
}

// WebhookUpdate Webhook 部分更新请求；指针字段为 nil 表示不修改。
type WebhookUpdate struct {
	Name       *string
	EventTypes []string
	TargetURL  *string
	Secret     *string
	Status     *string
}
