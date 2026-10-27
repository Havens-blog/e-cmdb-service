package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// genKey 生成 RSA 密钥对并导出公钥 PEM。
func genKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return key, string(pemBytes)
}

// signToken 用私钥签一个 RS256 JWT。
func signToken(t *testing.T, key *rsa.PrivateKey, exp time.Time) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   "e-cam-service",
		Issuer:    "https://iam.example.com",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func TestS2SAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, pubPEM := genKey(t)

	tests := []struct {
		name       string
		publicKey  string
		authHeader string
		wantStatus int
	}{
		{
			name:       "公钥未配置拒绝",
			publicKey:  "",
			authHeader: "Bearer whatever",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "缺少 Authorization 拒绝",
			publicKey:  pubPEM,
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "无效令牌拒绝",
			publicKey:  pubPEM,
			authHeader: "Bearer not-a-jwt",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "过期令牌拒绝",
			publicKey:  pubPEM,
			authHeader: "Bearer " + signToken(t, key, time.Now().Add(-time.Hour)),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "有效令牌放行",
			publicKey:  pubPEM,
			authHeader: "Bearer " + signToken(t, key, time.Now().Add(time.Hour)),
			wantStatus: http.StatusOK,
		},
		{
			name:       "错误公钥验签失败",
			publicKey:  func() string { _, p := genKey(t); return p }(),
			authHeader: "Bearer " + signToken(t, key, time.Now().Add(time.Hour)),
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/probe", S2SAuth(tt.publicKey), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
