package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

// RequireTenant 要求会话已选定租户。
//
// 与被它取代的旧实现有两处有意的差异：
//   - 状态码 400 → 403。旧模型下租户由客户端提供，缺失属参数问题；新模型下租户
//     来自会话，缺失意味着「未选定租户」，是权限问题。
//   - 判空 → 判零。租户类型已由 string 迁移为 int64。
func RequireTenant(logger *elog.Component) gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetTenantID(c) == 0 {
			if logger != nil {
				logger.Warn("会话未选定租户，拒绝访问",
					elog.String("path", c.Request.URL.Path))
			}
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "当前会话未选定租户空间",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
