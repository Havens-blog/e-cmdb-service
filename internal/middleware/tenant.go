package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
)

// TenantIDKey 租户ID在上下文中的键。
const TenantIDKey = "tenant_id"

// CtxUsernameKey 用户名在上下文中的键。
const CtxUsernameKey = "username"

// operatorCtxKey 操作者（用户名）在 context.Context 中的键。
// 用私有类型作键避免与其他包的字符串键冲突（service 层通过 OperatorFrom 读取，
// handler 层通过 WithOperator 注入，均在本包导出函数内完成）。
type operatorCtxKey struct{}

// WithOperator 把操作者用户名写入 context.Context（供 service 层审计使用）。
func WithOperator(ctx context.Context, operator string) context.Context {
	return context.WithValue(ctx, operatorCtxKey{}, operator)
}

// OperatorFrom 从 context.Context 读取操作者用户名（审计写入用）。
// 空串表示无操作者（如未登录的服务间调用/同步）。
func OperatorFrom(ctx context.Context) string {
	if v, ok := ctx.Value(operatorCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// SetTenantID 向上下文写入租户 ID（由认证中间件在解析会话后调用）。
func SetTenantID(c *gin.Context, tenantID int64) {
	c.Set(TenantIDKey, tenantID)
}

// GetTenantID 从上下文取出租户 ID。
// 返回 0 表示会话未选定租户（eiam 以 tenant_id=0 表示等待选择的临时凭证），
// 调用方不得把 0 当作「全部租户」通配，那会导致跨租户数据泄露。
func GetTenantID(c *gin.Context) int64 {
	if v, exists := c.Get(TenantIDKey); exists {
		if tid, ok := v.(int64); ok {
			return tid
		}
	}
	return 0
}

// GetUsername 从上下文中获取用户名。
func GetUsername(c *gin.Context) string {
	if username, exists := c.Get(CtxUsernameKey); exists {
		if u, ok := username.(string); ok {
			return u
		}
	}
	return ""
}
