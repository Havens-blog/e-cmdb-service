package web

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// TestOpenAPIErrorMessage 锁定错误到中文原因的映射。
//
// 之前非法输入由 gin 绑定直接拒成「空体 400」，前端只能弹通用文案；现在
// service 返回哨兵错误，由这里翻译成用户能看懂的原因，因此每条映射都要有
// 断言，避免以后改文案或改哨兵错误时静默退化成英文/内部错误。
func TestOpenAPIErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "empty token name", err: errs.ErrInvalidTokenName, want: "令牌名称不能为空"},
		{name: "token name too long", err: errs.ErrTokenNameTooLong, want: "令牌名称不能超过 64 个字符"},
		{name: "negative ttl", err: errs.ErrInvalidTokenTTL, want: "有效期天数不能为负数（填 0 表示永不过期）"},
		{name: "empty webhook name", err: errs.ErrInvalidWebhookName, want: "Webhook 名称不能为空"},
		{name: "webhook name too long", err: errs.ErrWebhookNameTooLong, want: "Webhook 名称不能超过 64 个字符"},
		{name: "empty target url", err: errs.ErrInvalidTargetURL, want: "回调地址不能为空"},
		{name: "target url too long", err: errs.ErrTargetURLTooLong, want: "回调地址不能超过 2048 个字符"},
		{name: "invalid event type", err: errs.ErrInvalidEventType, want: "事件类型无效，请至少选择一个受支持的事件类型"},
		{name: "invalid status", err: errs.ErrInvalidStatus, want: "状态无效，只支持 active 或 disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := openAPIErrorMessage(tc.err); got != tc.want {
				t.Fatalf("openAPIErrorMessage(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}

	t.Run("wrapped sentinel still mapped", func(t *testing.T) {
		wrapped := fmt.Errorf("create api token: %w", errs.ErrInvalidTokenTTL)
		if got := openAPIErrorMessage(wrapped); got != "有效期天数不能为负数（填 0 表示永不过期）" {
			t.Fatalf("wrapped error = %q, want the mapped message", got)
		}
	})

	t.Run("unknown error keeps original text", func(t *testing.T) {
		unknown := errors.New("mongo: connection refused")
		if got := openAPIErrorMessage(unknown); got != unknown.Error() {
			t.Fatalf("unknown error = %q, want original %q", got, unknown.Error())
		}
	})
}
