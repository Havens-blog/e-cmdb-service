package web

import (
	"net/url"
	"reflect"
	"testing"
)

// TestUnsupportedInstanceListParams 锁定「未知查询参数必须报错」。
//
// 背景（2026-11-17 事故）：实例列表接口此前静默忽略未知参数。调用方传
// keyword=zz-xxx 过滤后逐条删除，实际作用于全量列表前 N 条，误删 49 条真实实例。
// 本测试保证：白名单外的参数会被识别出来并返回给调用方，而不是被无声丢弃。
func TestUnsupportedInstanceListParams(t *testing.T) {
	t.Run("全部为已知参数时不报错", func(t *testing.T) {
		q := url.Values{
			"offset":     {"0"},
			"limit":      {"20"},
			"asset_name": {"web-01"},
			"model_uid":  {"ecs"},
			"tenant_id":  {"3"},
			"sort_by":    {"id"},
			"order":      {"desc"},
		}
		if got := unsupportedQueryParams(q, instanceListParamSet); len(got) != 0 {
			t.Fatalf("unknown = %v, want none", got)
		}
	})

	t.Run("拼错的过滤参数必须被报出来", func(t *testing.T) {
		if got := unsupportedQueryParams(url.Values{"bogus": {"x"}}, instanceListParamSet); !reflect.DeepEqual(got, []string{"bogus"}) {
			t.Fatalf("unknown = %v, want [bogus]", got)
		}
	})

	t.Run("多个未知参数按字典序返回", func(t *testing.T) {
		q := url.Values{"zz": {"1"}, "aa": {"2"}, "asset_name": {"ok"}}
		if got := unsupportedQueryParams(q, instanceListParamSet); !reflect.DeepEqual(got, []string{"aa", "zz"}) {
			t.Fatalf("unknown = %v, want [aa zz]", got)
		}
	})

	t.Run("keyword 是已知参数（第223轮别名）", func(t *testing.T) {
		if got := unsupportedQueryParams(url.Values{"keyword": {"zz-p223-"}}, instanceListParamSet); len(got) != 0 {
			t.Fatalf("keyword 应被接受，实际 unknown = %v", got)
		}
	})

	t.Run("属性筛选参数均被接受", func(t *testing.T) {
		q := url.Values{}
		for _, key := range instanceListAttributeParams {
			q.Set(key, "v")
		}
		if got := unsupportedQueryParams(q, instanceListParamSet); len(got) != 0 {
			t.Fatalf("属性参数应被接受，实际 unknown = %v", got)
		}
	})

	t.Run("空查询不被判为未知", func(t *testing.T) {
		if got := unsupportedQueryParams(url.Values{}, instanceListParamSet); len(got) != 0 {
			t.Fatalf("unknown = %v, want none", got)
		}
	})
}

// TestResolveInstanceModelFilter 锁定 uid → model_uid 别名真的生效。
//
// 白名单接受 uid，若后端不消费它就是「接受但忽略」的第二种静默失败：
// 关系页资产搜索传 uid=ecs 时会拿到全量列表。
func TestResolveInstanceModelFilter(t *testing.T) {
	cases := []struct {
		name     string
		modelUID string
		uid      string
		want     string
	}{
		{"只传 model_uid", "ecs", "", "ecs"},
		{"只传 uid 时作为别名生效", "", "ecs", "ecs"},
		{"同时传入以 model_uid 为准", "ecs", "disk", "ecs"},
		{"uid 首尾空格被裁剪", "", "  ecs  ", "ecs"},
		{"model_uid 为纯空格时退回 uid", "   ", "ecs", "ecs"},
		{"两者都为空时不加模型过滤", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveInstanceModelFilter(tc.modelUID, tc.uid); got != tc.want {
				t.Fatalf("resolveInstanceModelFilter(%q, %q) = %q, want %q", tc.modelUID, tc.uid, got, tc.want)
			}
		})
	}
}

// TestInstanceListAttributeParams 锁定「属性参数会真的参与 attributes 过滤」。//
// 白名单只是第一步；参数被接受却仍不生效同样是静默失败，
// 所以这里同时断言属性清单与白名单一致，防止两者被改歪。
func TestInstanceListAttributeParams(t *testing.T) {
	if len(instanceListAttributeParams) == 0 {
		t.Fatal("属性参数清单不应为空")
	}
	for _, key := range instanceListAttributeParams {
		if _, ok := instanceListParamSet[key]; !ok {
			t.Fatalf("属性参数 %q 不在白名单内，会被判为未知参数", key)
		}
	}
	// 状态/区域/云平台历史上就直接映射，不在动态清单里，但必须在白名单内。
	for _, key := range []string{"status", "region", "provider"} {
		if _, ok := instanceListParamSet[key]; !ok {
			t.Fatalf("%q 不在白名单内", key)
		}
	}
}
