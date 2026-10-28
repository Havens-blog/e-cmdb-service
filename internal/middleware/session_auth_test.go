package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ecodeclub/ekit"
	"github.com/ecodeclub/ginx/gctx"
	"github.com/ecodeclub/ginx/session"
	"github.com/gin-gonic/gin"
)

// fakeSession 实现 session.Session，仅提供 Claims（其余方法测试用不到）。
type fakeSession struct{ claims session.Claims }

func (f *fakeSession) Set(context.Context, string, any) error    { return nil }
func (f *fakeSession) Get(context.Context, string) ekit.AnyValue { return ekit.AnyValue{} }
func (f *fakeSession) Del(context.Context, string) error         { return nil }
func (f *fakeSession) Destroy(context.Context) error             { return nil }
func (f *fakeSession) Claims() session.Claims                    { return f.claims }

// fakeProvider 实现 session.Provider，Get 返回预置结果。
type fakeProvider struct {
	sess session.Session
	err  error
}

func (f *fakeProvider) NewSession(*gctx.Context, int64, map[string]string, map[string]any) (session.Session, error) {
	return nil, errors.New("not supported")
}
func (f *fakeProvider) Get(*gctx.Context) (session.Session, error) { return f.sess, f.err }
func (f *fakeProvider) Destroy(*gctx.Context) error                { return nil }
func (f *fakeProvider) UpdateClaims(*gctx.Context, session.Claims) error {
	return nil
}
func (f *fakeProvider) RenewAccessToken(*gctx.Context) error { return nil }

func claimsWith(data map[string]string) session.Claims {
	return session.Claims{Uid: 42, SSID: "ssid-1", Data: data}
}

func TestSessionAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		provider   *fakeProvider
		headers    map[string]string
		wantStatus int
		wantTenant int64
		wantUser   string
	}{
		{
			name:       "无会话拒绝 401",
			provider:   &fakeProvider{err: errors.New("no session")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "正常会话注入租户与用户名",
			provider: &fakeProvider{sess: &fakeSession{claims: claimsWith(map[string]string{
				"tenant_id": "7", "username": "alice",
			})}},
			wantStatus: http.StatusOK,
			wantTenant: 7,
			wantUser:   "alice",
		},
		{
			name: "会话未选租户放行但租户为 0(交给 RequireTenant)",
			provider: &fakeProvider{sess: &fakeSession{claims: claimsWith(map[string]string{
				"username": "alice",
			})}},
			wantStatus: http.StatusOK,
			wantTenant: 0,
			wantUser:   "alice",
		},
		{
			name: "客户端伪造 X-Tenant-Id 被忽略,以会话为准",
			provider: &fakeProvider{sess: &fakeSession{claims: claimsWith(map[string]string{
				"tenant_id": "7",
			})}},
			headers:    map[string]string{"X-Tenant-Id": "999"},
			wantStatus: http.StatusOK,
			wantTenant: 7,
		},
		{
			name:       "无会话时伪造 X-Tenant-Id 也拒绝(不再存在头透传旁路)",
			provider:   &fakeProvider{err: errors.New("no session")},
			headers:    map[string]string{"X-Tenant-Id": "999", "X-Request-User": "attacker"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "tenant_id 非法不崩溃,租户为 0",
			provider: &fakeProvider{sess: &fakeSession{claims: claimsWith(map[string]string{
				"tenant_id": "abc",
			})}},
			wantStatus: http.StatusOK,
			wantTenant: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			var gotTenant int64
			var gotUser string
			router.GET("/probe", SessionAuth(tt.provider, nil), func(c *gin.Context) {
				gotTenant = GetTenantID(c)
				gotUser = GetUsername(c)
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				if gotTenant != tt.wantTenant {
					t.Errorf("tenant = %d, want %d", gotTenant, tt.wantTenant)
				}
				if gotUser != tt.wantUser {
					t.Errorf("username = %q, want %q", gotUser, tt.wantUser)
				}
			}
		})
	}
}
