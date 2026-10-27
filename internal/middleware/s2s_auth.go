package middleware

import (
	"crypto/rsa"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// S2SAuth 服务间调用鉴权中间件：校验 eiam 签发的 RS256 JWT 的服务身份。
//
// 与 eiam 侧 client_credentials 签发、本服务 /internal 端点配套：
// 只确认"调用方持有 eiam 签发的有效服务令牌"（eiam 唯一签发方），
// 不强制租户——本服务 /internal 端点租户由请求显式传参（见 internal_api.go）。
//
// fail-closed：公钥未配置 / 无令牌 / 验签失败 / 令牌过期 一律 401。
func S2SAuth(publicKeyPEM string) gin.HandlerFunc {
	pub := parseRSAPublicKey(publicKeyPEM)
	return func(c *gin.Context) {
		if pub == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "服务间鉴权公钥未配置或无效"})
			c.Abort()
			return
		}

		authHeader := c.GetHeader("Authorization")
		tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "缺少服务令牌"})
			c.Abort()
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodRS256 {
				return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
			}
			return pub, nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "服务令牌无效或已过期"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// parseRSAPublicKey 构造期解析一次公钥，失败返回 nil（中间件将 fail-closed 拒绝）。
func parseRSAPublicKey(pem string) *rsa.PublicKey {
	if strings.TrimSpace(pem) == "" {
		return nil
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pem))
	if err != nil {
		return nil
	}
	return pub
}
