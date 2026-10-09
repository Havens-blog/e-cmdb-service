package middleware

import "github.com/gin-gonic/gin"

// TenantIDKey 租户ID在上下文中的键。
const TenantIDKey = "tenant_id"

// CtxUsernameKey 用户名在上下文中的键。
const CtxUsernameKey = "username"

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
