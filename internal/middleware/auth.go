package middleware

import (
	"net/http"
	"strconv"

	"github.com/ecodeclub/ginx/gctx"
	"github.com/ecodeclub/ginx/session"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

// SessionAuth 本地校验平台共享会话（eiam 签发 HS256 会话 JWT，密钥与 Redis
// 存储与 eiam/e-cam-service 共享），从会话 claims 解析租户与操作者写入上下文。
//
// 与 e-cam-service 的 EcmdbAuthMiddleware 同口径：租户身份以会话 claims 为唯一
// 权威，客户端自报的 X-Tenant-Id 头一律不采纳。
//
// 历史教训：旧实现 GatewayAuth 信任网关透传的 X-Tenant-Id 头，但网关（nginx）
// 既不做会话校验也不剥离客户端同名头，且该头实际由前端自填——等于任何直连
// 请求都能伪造租户读写他租户 CMDB 数据。本中间件为修复该漏洞而来，
// 不得再引入任何"信任客户端自报租户"的路径。
//
// fail-closed：无会话/验签失败/过期一律 401。会话未选定租户（claims 缺
// tenant_id 或为 0）时本中间件放行但不写租户，由下游 RequireTenant 统一拒绝
// （0 永不通配，防跨租户泄露）。
func SessionAuth(sp session.Provider, logger *elog.Component) gin.HandlerFunc {
	return func(c *gin.Context) {
		gCtx := &gctx.Context{Context: c}
		sess, err := sp.Get(gCtx)
		if err != nil {
			if logger != nil {
				logger.Debug("会话认证失败",
					elog.FieldErr(err),
					elog.String("client_ip", c.ClientIP()),
					elog.String("path", c.Request.URL.Path))
			}
			AbortUnauthorized(c)
			return
		}

		claims := sess.Claims()
		if username, ok := claims.Data["username"]; ok && username != "" {
			c.Set(CtxUsernameKey, username)
		}

		// claims 里的 tenant_id 是字符串（eiam 侧写入），解析为 int64；
		// 解析失败或缺失时不设置该键，GetTenantID 返回 0，由 RequireTenant 拒绝。
		if raw, ok := claims.Data["tenant_id"]; ok && raw != "" {
			if tid, err := strconv.ParseInt(raw, 10, 64); err == nil {
				SetTenantID(c, tid)
			} else if logger != nil {
				logger.Warn("会话中的 tenant_id 无法解析为 int64",
					elog.String("raw", raw),
					elog.String("path", c.Request.URL.Path))
			}
		}

		// 保存 session 到上下文，供后续中间件/handler 复用（避免重复解析）
		c.Set(session.CtxSessionKey, sess)
		c.Next()
	}
}

// AbortUnauthorized 统一 401 响应。
func AbortUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "认证失败：会话无效或已过期"})
	c.Abort()
}
