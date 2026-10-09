package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

// 网关透传的可信身份头。e-cmdb-service 作为内网微服务置于运维平台网关之后，
// 由网关完成 session/JWT 校验后，将解析出的租户与用户名以请求头透传。
// 直连（绕过网关）时这些头缺失，租户为 0，被 RequireTenant 拦截。
const (
	// HeaderTenantID 网关透传的租户 ID 头。
	HeaderTenantID = "X-Tenant-Id"
	// HeaderUsername 网关透传的操作者用户名头。
	HeaderUsername = "X-Request-User"
)

// GatewayAuth 从网关透传头解析租户与操作者并写入上下文。
//
// 安全前提：本服务只在内网监听，且仅接受来自运维平台网关的流量；
// X-Tenant-Id 由网关在校验会话后注入，外部不可伪造。若未来需要直连校验，
// 在此处替换为 session/JWT 解析逻辑即可，下游 handler 无需改动。
func GatewayAuth(logger *elog.Component) gin.HandlerFunc {
	return func(c *gin.Context) {
		if raw := c.GetHeader(HeaderTenantID); raw != "" {
			if tid, err := strconv.ParseInt(raw, 10, 64); err == nil {
				SetTenantID(c, tid)
			} else if logger != nil {
				logger.Warn("网关租户头解析失败",
					elog.String("raw", raw),
					elog.String("path", c.Request.URL.Path))
			}
		}
		if user := c.GetHeader(HeaderUsername); user != "" {
			c.Set(CtxUsernameKey, user)
		}
		c.Next()
	}
}

// AbortUnauthorized 统一 401 响应（预留给未来直连校验失败路径）。
func AbortUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未认证"})
	c.Abort()
}
