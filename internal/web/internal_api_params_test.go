package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestParseIntQuery 锁定内部 S2S 端点的整数参数解析口径：
// 缺省/空串 → 沿用历史默认值；显式非法值 → 必须报错（不得静默变 0）。
//
// 关键回归点：tenant_id 的 0 在 DAO 里表示「不加租户过滤」，
// 旧实现 `v, _ := strconv.ParseInt(...)` 会把 tenant_id=abc 变成 0，
// 于是一次拼错参数就变成跨租户全量读取（2026-11-19 线上实测 total=21498）。
func TestParseIntQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name        string
		query       string
		wantValue   int64
		wantPresent bool
		wantErr     bool
	}{
		{name: "缺省参数", query: "", wantValue: 0, wantPresent: false},
		{name: "空串视为未传", query: "?tenant_id=", wantValue: 0, wantPresent: false},
		{name: "纯空格视为未传", query: "?tenant_id=%20%20", wantValue: 0, wantPresent: false},
		{name: "正常整数", query: "?tenant_id=3", wantValue: 3, wantPresent: true},
		{name: "零是合法整数", query: "?tenant_id=0", wantValue: 0, wantPresent: true},
		{name: "负数是合法整数", query: "?tenant_id=-1", wantValue: -1, wantPresent: true},
		{name: "首尾空格裁剪", query: "?tenant_id=%203%20", wantValue: 3, wantPresent: true},
		{name: "字母必须报错", query: "?tenant_id=abc", wantErr: true},
		{name: "数字混字母必须报错", query: "?tenant_id=3abc", wantErr: true},
		{name: "小数必须报错", query: "?tenant_id=3.5", wantErr: true},
		{name: "超长整数必须报错", query: "?tenant_id=99999999999999999999999", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := httptest.NewRequest(http.MethodGet, "/internal/instances/list"+tc.query, http.NoBody)
			c.Request = req
			value, present, errMsg := parseIntQuery(c, "tenant_id")
			if tc.wantErr {
				if errMsg == "" {
					t.Fatalf("期望报错，实际放行（value=%d present=%v）", value, present)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("不应报错，实际 errMsg=%q", errMsg)
			}
			if value != tc.wantValue || present != tc.wantPresent {
				t.Fatalf("value=%d present=%v，期望 value=%d present=%v", value, present, tc.wantValue, tc.wantPresent)
			}
		})
	}
}

// TestClampInternalLimit 锁定单次取数上限：
// DAO 对 limit<=0 不加 $limit（等于导出全集合，实测未绑定实例 20166 条 / 9.2 秒），
// 所以 0、负数、缺省都必须变成默认值，超上限钳到上限而不是回落默认值。
func TestClampInternalLimit(t *testing.T) {
	cases := []struct {
		name    string
		limit   int64
		present bool
		want    int64
	}{
		{name: "缺省取默认", limit: 0, present: false, want: internalListDefaultLimit},
		{name: "显式零取默认", limit: 0, present: true, want: internalListDefaultLimit},
		{name: "负数取默认", limit: -3, present: true, want: internalListDefaultLimit},
		{name: "正常值原样保留", limit: 5, present: true, want: 5},
		{name: "正好上限保留", limit: 500, present: true, want: 500},
		{name: "超上限钳到上限", limit: 501, present: true, want: internalListMaxLimit},
		{name: "超大值钳到上限", limit: 1000000, present: true, want: internalListMaxLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampInternalLimit(tc.limit, tc.present); got != tc.want {
				t.Fatalf("clampInternalLimit(%d, %v)=%d，期望 %d", tc.limit, tc.present, got, tc.want)
			}
		})
	}
}

// TestInternalAPIRejectsMalformedParams 走真实路由，验证非法参数在触达仓储之前就被 400 拦下。
// handler 的 repo 为 nil：只要有一个请求往下走到仓储调用就会 panic，因此这个测试同时
// 证明「拒绝发生在取数之前」。
func TestInternalAPIRejectsMalformedParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &InternalAPIHandler{}
	engine := gin.New()
	h.RegisterRoutes(engine.Group("/api/v1/cmdb"))

	cases := []struct {
		name string
		path string
	}{
		{name: "list tenant_id 非法", path: "/api/v1/cmdb/internal/instances/list?tenant_id=abc"},
		{name: "list limit 非法", path: "/api/v1/cmdb/internal/instances/list?tenant_id=3&limit=abc"},
		{name: "unbound tenant_id 非法", path: "/api/v1/cmdb/internal/instances/unbound?tenant_id=abc"},
		{name: "unbound limit 非法", path: "/api/v1/cmdb/internal/instances/unbound?tenant_id=3&limit=abc"},
		{name: "unbound offset 负数", path: "/api/v1/cmdb/internal/instances/unbound?tenant_id=3&offset=-5"},
		{name: "unbound count tenant_id 非法", path: "/api/v1/cmdb/internal/instances/unbound/count?tenant_id=abc"},
		{name: "stats all tenant_id 非法", path: "/api/v1/cmdb/internal/stats/all?tenant_id=abc"},
		{name: "stats unbound tenant_id 非法", path: "/api/v1/cmdb/internal/stats/unbound?tenant_id=abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, http.NoBody)
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("状态码 %d，期望 400（body=%s）", w.Code, w.Body.String())
			}
			if body := w.Body.String(); body == "" || body == "{}" {
				t.Fatalf("400 响应必须带可读原因，实际 body=%q", body)
			}
		})
	}
}
