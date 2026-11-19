package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
)

// TestUnsupportedQueryParams 锁定共享守卫的核心行为。
func TestUnsupportedQueryParams(t *testing.T) {
	allowed := newParamSet("offset", "limit", "provider")

	t.Run("全部已知时返回空", func(t *testing.T) {
		got := unsupportedQueryParams(map[string][]string{"offset": {"0"}, "limit": {"20"}}, allowed)
		if len(got) != 0 {
			t.Fatalf("unknown = %v, want none", got)
		}
	})

	t.Run("未知参数按字典序返回", func(t *testing.T) {
		got := unsupportedQueryParams(map[string][]string{"zz": {"1"}, "aa": {"2"}, "limit": {"5"}}, allowed)
		if !reflect.DeepEqual(got, []string{"aa", "zz"}) {
			t.Fatalf("unknown = %v, want [aa zz]", got)
		}
	})

	t.Run("同一参数重复出现只报一次", func(t *testing.T) {
		got := unsupportedQueryParams(map[string][]string{"bogus": {"1", "2", "3"}}, allowed)
		if !reflect.DeepEqual(got, []string{"bogus"}) {
			t.Fatalf("unknown = %v, want [bogus]", got)
		}
	})

	t.Run("空字符串值也算传了该参数", func(t *testing.T) {
		// url.Values 对 `?bogus=` 会给出 {"bogus": {""}}，必须报错而不是当作没传。
		got := unsupportedQueryParams(map[string][]string{"bogus": {""}}, allowed)
		if !reflect.DeepEqual(got, []string{"bogus"}) {
			t.Fatalf("unknown = %v, want [bogus]", got)
		}
	})
}

// TestRejectUnsupportedQuery_WritesEnvelope 校验 400 响应契约。
func TestRejectUnsupportedQuery_WritesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(ctx *gin.Context) {
		if rejectUnsupportedQuery(ctx, newParamSet("limit")) {
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"ok": true})
	})

	t.Run("未知参数 → 400 + ParamsError 信封", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?limit=1&bogus=2&aa=3", nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		var body struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
		}
		if body.Code == 0 {
			t.Fatalf("code = %d, want non-zero", body.Code)
		}
		for _, want := range []string{"unsupported query parameter(s):", "aa", "bogus"} {
			if !strings.Contains(body.Msg, want) {
				t.Fatalf("msg = %q, want contain %q", body.Msg, want)
			}
		}
	})

	t.Run("全部已知时放行到处理器", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?limit=1", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

// TestGuardQueryParams_ReturnsResult 校验返回形态版本。
func TestGuardQueryParams_ReturnsResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x?bogus=1", nil)

	res, bad := guardQueryParams(c, newParamSet("limit"))
	if !bad {
		t.Fatalf("bad = false, want true")
	}
	if res.Code == 0 || !strings.Contains(res.Msg, "bogus") {
		t.Fatalf("res = %+v, want params error mentioning bogus", res)
	}

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodGet, "/x?limit=1", nil)
	if _, bad2 := guardQueryParams(c2, newParamSet("limit")); bad2 {
		t.Fatalf("bad = true for allowed params")
	}
}

// TestListParamSetsMatchHandlerReads 锁定「白名单 = 处理器真正读取的参数」。
//
// 这张表是刻意硬编码的：处理器新增一个 ctx.Query("x") 而忘了加进白名单，
// 会让新参数被 400 拒绝（这里会失败）；反过来白名单里塞了处理器不读的参数，
// 会重新制造「接受但不生效」的静默陷阱（同样在这里失败）。
func TestListParamSetsMatchHandlerReads(t *testing.T) {
	cases := []struct {
		name    string
		set     paramSet
		mustHas []string
	}{
		{
			name:    "models",
			set:     modelListParamSet,
			mustHas: []string{"offset", "limit", "provider", "category", "parent_uid", "level", "model_group_id", "extensible"},
		},
		{
			name:    "model-groups",
			set:     modelGroupListParamSet,
			mustHas: []string{"offset", "limit"},
		},
		{
			name:    "attributes",
			set:     attributeListParamSet,
			mustHas: []string{"offset", "limit", "group_id", "field_type", "required", "display", "searchable"},
		},
		{
			name:    "model-relations",
			set:     modelRelationListParamSet,
			mustHas: []string{"offset", "limit", "source_model_uid", "target_model_uid", "relation_type"},
		},
		{
			name:    "instance-relations",
			set:     instanceRelationListParamSet,
			mustHas: []string{"offset", "limit", "source_instance_id", "target_instance_id", "relation_type_uid"},
		},
		{
			name:    "quality-issues",
			set:     qualityIssuesParamSet,
			mustHas: []string{"offset", "limit", "type"},
		},
		{
			name:    "quality-field-issues",
			set:     qualityFieldIssuesParamSet,
			mustHas: []string{"offset", "limit", "model_uid", "issue_type"},
		},
		{
			name:    "quality-sync-issues",
			set:     qualitySyncIssuesParamSet,
			mustHas: []string{"offset", "limit", "model_uid", "issue_type", "asset_id"},
		},
		{
			name:    "impact",
			set:     impactParamSet,
			mustHas: []string{"instance_id", "depth"},
		},
		{
			name:    "topology-instance",
			set:     instanceTopologyParamSet,
			mustHas: []string{"depth", "direction", "model_uid"},
		},
		{
			name:    "topology-model",
			set:     modelTopologyParamSet,
			mustHas: []string{"provider"},
		},
		{
			name:    "topology-related",
			set:     relatedInstancesParamSet,
			mustHas: []string{"relation_type_uid"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.set) != len(tc.mustHas) {
				t.Fatalf("set size = %d, want %d, set=%v", len(tc.set), len(tc.mustHas), tc.set)
			}
			for _, key := range tc.mustHas {
				if _, ok := tc.set[key]; !ok {
					t.Fatalf("missing allowed param %q", key)
				}
			}
		})
	}

	t.Run("页面专用 query 键不得混入接口白名单", func(t *testing.T) {
		// page/size 是前端列表分页的 URL 键（换算成 offset/limit 后才发请求），
		// instance_role 是关系页的 URL 键（换算成 source/target_instance_id）。
		for _, key := range []string{"page", "size", "instance_role"} {
			if _, ok := instanceRelationListParamSet[key]; ok {
				t.Fatalf("page-only key %q leaked into instance-relations whitelist", key)
			}
			if _, ok := modelListParamSet[key]; ok {
				t.Fatalf("page-only key %q leaked into models whitelist", key)
			}
		}
	})
}

// TestListEndpointsRejectUnknownParamEndToEnd 走真实 gin 路由，证明拒绝发生在取数之前
// （处理器持有的 svc 为 nil —— 一旦越过守卫就会 nil panic）。
//
// 注意响应形态在当前代码库里并不统一，这里按处理器族的既有约定分别断言：
//   - void 族处理器（自己 ctx.JSON）→ HTTP 400
//   - (ginx.Result, error) 族处理器（经 ginx.Wrap 包装）→ HTTP 200 + 信封 code 非 0
//
// 这个差异是历史遗留（见 docs/cmdb-agent-iterations.md 待确认项），本轮只统一
// 「未知参数必须被拒绝」，不改变各端点族的响应形态。
func TestListEndpointsRejectUnknownParamEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	mh := &ModelHandler{}
	gh := &ModelGroupHandler{}
	ah := &AttributeHandler{}
	rh := &RelationHandler{}
	qh := &QualityHandler{}
	ih := &ImpactHandler{}

	r.GET("/models", mh.List)
	r.GET("/model-groups", gh.List)
	r.GET("/models/:uid/attributes", ah.ListAttributes)
	r.GET("/model-relations", rh.ListModelRelations)
	r.GET("/instance-relations", rh.ListInstanceRelations)
	r.GET("/quality/issues", ginx.Wrap(qh.Issues))
	r.GET("/quality/field-issues", ginx.Wrap(qh.FieldIssues))
	r.GET("/quality/sync-issues", ginx.Wrap(qh.SyncIssues))
	r.GET("/impact", ginx.Wrap(ih.GetImpact))

	http400 := []string{
		"/models", "/model-groups", "/models/ecs/attributes",
		"/model-relations", "/instance-relations",
	}
	envelope := []string{
		"/quality/issues", "/quality/field-issues", "/quality/sync-issues", "/impact",
	}

	for _, p := range http400 {
		p := p
		t.Run(p, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p+"?bogus=1", nil))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("%s: status = %d, want 400 (body=%s)", p, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "bogus") {
				t.Fatalf("%s: body = %s, want mention bogus", p, w.Body.String())
			}
		})
	}

	for _, p := range envelope {
		p := p
		t.Run(p, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p+"?bogus=1", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, want 200 envelope (body=%s)", p, w.Code, w.Body.String())
			}
			var body struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s: unmarshal %v body=%s", p, err, w.Body.String())
			}
			if body.Code == 0 {
				t.Fatalf("%s: code = 0, want non-zero", p)
			}
			if !strings.Contains(body.Msg, "bogus") {
				t.Fatalf("%s: msg = %q, want mention bogus", p, body.Msg)
			}
		})
	}

}
