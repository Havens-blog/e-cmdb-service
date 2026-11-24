package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// TestParseOptionalBoolParam 锁定布尔参数的三态语义：未传 / true / false，
// 且「无法识别的值」必须报错而不是退化成「未传」。
func TestParseOptionalBoolParam(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantNil   bool
		wantValue bool
		wantErr   bool
	}{
		{name: "未传", raw: "", wantNil: true},
		{name: "纯空格视为未传", raw: "   ", wantNil: true},
		{name: "true", raw: "true", wantValue: true},
		{name: "TRUE 大小写不敏感", raw: "TRUE", wantValue: true},
		{name: "1", raw: "1", wantValue: true},
		{name: "带空格", raw: " true ", wantValue: true},
		{name: "false", raw: "false", wantValue: false},
		{name: "0", raw: "0", wantValue: false},
		{name: "无法识别必须报错", raw: "zzz", wantErr: true},
		{name: "yes 不在白名单", raw: "yes", wantErr: true},
		{name: "2 不是布尔", raw: "2", wantErr: true},
		{name: "-1 不是布尔", raw: "-1", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, errMsg := parseOptionalBoolParam(tc.raw, "required")
			if tc.wantErr {
				if errMsg == "" {
					t.Fatalf("raw=%q: 期望报错，实际放行（got=%v）", tc.raw, got)
				}
				// 报错必须能定位：既说清参数名，也回显收到的值。
				if !strings.Contains(errMsg, "required") || !strings.Contains(errMsg, tc.raw) {
					t.Fatalf("errMsg = %q, 期望同时包含参数名与原始值", errMsg)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("raw=%q: 意外报错 %q", tc.raw, errMsg)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("raw=%q: 期望 nil（未传），实际 %v", tc.raw, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("raw=%q: 期望 %v，实际 nil", tc.raw, tc.wantValue)
			}
			if *got != tc.wantValue {
				t.Fatalf("raw=%q: 期望 %v，实际 %v", tc.raw, tc.wantValue, *got)
			}
		})
	}
}

// TestValidateEnumParam 锁定枚举参数校验：未传放行、命中放行并返回归一化值、其余报错并列出合法值。
//
// 返回值本身就是防回归点：调用方必须使用归一化值，否则 '?type=%20orphan_instance'
// 会通过校验却在下游精确比较时匹配不到任何类型（静默空集）。
func TestValidateEnumParam(t *testing.T) {
	allowed := []string{"orphan_instance", "missing_field", "dangling_relation"}

	for _, unset := range []string{"", "   ", "\t"} {
		got, msg := validateEnumParam(unset, "type", allowed)
		if msg != "" {
			t.Fatalf("raw=%q 应视为未传，实际报错 %q", unset, msg)
		}
		if got != "" {
			t.Fatalf("raw=%q 归一化后应为空串，实际 %q", unset, got)
		}
	}

	for _, ok := range allowed {
		got, msg := validateEnumParam(ok, "type", allowed)
		if msg != "" {
			t.Fatalf("合法值 %q 被拒绝：%q", ok, msg)
		}
		if got != ok {
			t.Fatalf("合法值 %q 归一化后应原样返回，实际 %q", ok, got)
		}
	}

	t.Run("首尾空格被归一化而不是透传", func(t *testing.T) {
		got, msg := validateEnumParam("  orphan_instance  ", "type", allowed)
		if msg != "" {
			t.Fatalf("带空格但仍合法，不应报错，实际 %q", msg)
		}
		if got != "orphan_instance" {
			t.Fatalf("归一化后应为 orphan_instance，实际 %q", got)
		}
	})

	got, msg := validateEnumParam("zzz", "type", allowed)
	if msg == "" {
		t.Fatal("未知枚举值应报错")
	}
	if got != "" {
		t.Fatalf("报错时归一化值应为空串，实际 %q", got)
	}
	for _, want := range append([]string{"type", "zzz"}, allowed...) {
		if !strings.Contains(msg, want) {
			t.Fatalf("errMsg = %q, 缺少 %q", msg, want)
		}
	}
}

// TestEnumSetsMatchDownstream 锁定枚举集合与下游实现一致。
//
// 集合写错的方向有两种，都会骗调用方：
//   - 漏了合法值 → 正常筛选被 400；
//   - 多了不存在的值 → 该值能通过校验但永远返回空集（等于没修）。
func TestEnumSetsMatchDownstream(t *testing.T) {
	t.Run("field-issues 只接受字段级两类", func(t *testing.T) {
		want := []string{service.QualityIssueTypeFieldType, service.QualityIssueTypeFieldEnum}
		if len(qualityFieldIssueTypes) != len(want) {
			t.Fatalf("qualityFieldIssueTypes = %v, want %v", qualityFieldIssueTypes, want)
		}
		for i := range want {
			if qualityFieldIssueTypes[i] != want[i] {
				t.Fatalf("qualityFieldIssueTypes = %v, want %v", qualityFieldIssueTypes, want)
			}
		}
	})

	t.Run("issues 只接受判定级三类", func(t *testing.T) {
		want := []string{
			service.QualityIssueTypeOrphan,
			service.QualityIssueTypeMissingField,
			service.QualityIssueTypeDanglingRelation,
		}
		if len(qualityIssueTypes) != len(want) {
			t.Fatalf("qualityIssueTypes = %v, want %v", qualityIssueTypes, want)
		}
		for _, w := range want {
			found := false
			for _, got := range qualityIssueTypes {
				if got == w {
					found = true
				}
			}
			if !found {
				t.Fatalf("qualityIssueTypes 缺少 %q", w)
			}
		}
		// 字段级问题在 fieldIssues 里，放在这里只会得到空集。
		for _, bad := range []string{service.QualityIssueTypeFieldType, service.QualityIssueTypeFieldEnum} {
			for _, got := range qualityIssueTypes {
				if got == bad {
					t.Fatalf("qualityIssueTypes 不应包含字段级类型 %q", bad)
				}
			}
		}
	})

	t.Run("sync-issues 接受 ValidateAttributes 的三种警告", func(t *testing.T) {
		// 取值来源：internal/domain/instance_validation.go 里 Warning.Type 的
		// "required" / "enum" / "type"，recordSyncQualityIssues 原样写入 IssueType。
		want := []string{"required", "enum", "type"}
		if len(syncIssueTypes) != len(want) {
			t.Fatalf("syncIssueTypes = %v, want %v", syncIssueTypes, want)
		}
		for _, w := range want {
			found := false
			for _, got := range syncIssueTypes {
				if got == w {
					found = true
				}
			}
			if !found {
				t.Fatalf("syncIssueTypes 缺少 %q", w)
			}
		}
	})
}

// TestParamValueRejectionsEndToEnd 走真实 gin 路由，证明「值非法」在取数之前被拒绝
// （处理器持有的 svc 为 nil —— 一旦越过校验就会 nil panic）。
func TestParamValueRejectionsEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	mh := &ModelHandler{}
	ah := &AttributeHandler{}
	qh := &QualityHandler{}

	r.GET("/models", mh.List)
	r.GET("/models/:uid/attributes", ah.ListAttributes)
	r.GET("/quality/issues", ginx.Wrap(qh.Issues))
	r.GET("/quality/field-issues", ginx.Wrap(qh.FieldIssues))
	r.GET("/quality/sync-issues", ginx.Wrap(qh.SyncIssues))
	r.GET("/quality/overview", ginx.Wrap(qh.Overview))
	r.GET("/quality/orphan-sources", ginx.Wrap(qh.OrphanSources))

	// void 族：HTTP 400 + 信封
	http400 := []struct {
		path string
		want []string
	}{
		{"/models?extensible=zzz", []string{"extensible", "zzz", "true/false/1/0"}},
		{"/models/ecs/attributes?required=zzz", []string{"required", "zzz"}},
		{"/models/ecs/attributes?display=zzz", []string{"display", "zzz"}},
		{"/models/ecs/attributes?searchable=zzz", []string{"searchable", "zzz"}},
	}
	for _, tc := range http400 {
		tc := tc
		t.Run("400 "+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(w.Body.String(), want) {
					t.Fatalf("body = %s, 缺少 %q", w.Body.String(), want)
				}
			}
		})
	}

	// 信封族：HTTP 200 + code 非 0（沿用该端点族既有约定）
	envelope := []struct {
		path string
		want []string
	}{
		{"/quality/issues?type=zzz", []string{"type", "zzz", "orphan_instance", "field-issues"}},
		{"/quality/field-issues?issue_type=zzz", []string{"issue_type", "zzz", "field_enum"}},
		{"/quality/sync-issues?issue_type=zzz", []string{"issue_type", "zzz", "required"}},
		// 这两个端点不接受任何参数：此前 ?limit=abc、?offset=1&limit=1 被静默忽略
		// 并返回完整数据，调用方会误以为自己传的分页生效了。
		{"/quality/overview?limit=abc", []string{"limit"}},
		{"/quality/orphan-sources?offset=1&limit=1", []string{"offset", "limit"}},
	}
	for _, tc := range envelope {
		tc := tc
		t.Run("信封 "+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 envelope (body=%s)", w.Code, w.Body.String())
			}
			var body struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal %v body=%s", err, w.Body.String())
			}
			if body.Code == 0 {
				t.Fatalf("code = 0, want non-zero")
			}
			for _, want := range tc.want {
				if !strings.Contains(body.Msg, want) {
					t.Fatalf("msg = %q, 缺少 %q", body.Msg, want)
				}
			}
		})
	}
}

// TestParamValueEmptyMeansUnsetEndToEnd 确认「清空筛选」不会被新校验打断：
// `?required=` / `?type=` 这类空值仍走「未传」分支，不会变成 400。
//
// 处理器会在校验通过后访问 nil svc，因此这里只断言「没有返回 400/参数错误」，
// 用 recover 接住 nil 解引用——只要不是 400 就说明空值被正确当作未传。
func TestParamValueEmptyMeansUnsetEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/models/ecs/attributes", (&AttributeHandler{}).ListAttributes)

	for _, path := range []string{
		"/models/ecs/attributes?required=",
		"/models/ecs/attributes?display=&searchable=",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			func() {
				defer func() { _ = recover() }() // nil svc 引发的 panic 与本节断言无关
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			}()
			if w.Code == http.StatusBadRequest {
				t.Fatalf("空值被当成非法值拒绝了: body=%s", w.Body.String())
			}
		})
	}
}
