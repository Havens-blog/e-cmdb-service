package web

import "testing"

func TestResolvePaging_Defaults(t *testing.T) {
	offset, limit, errMsg := resolvePaging("", "")
	if errMsg != "" || offset != 0 || limit != defaultListLimit {
		t.Fatalf("缺省分页应为 (0,%d)，实际 (%d,%d) err=%q", defaultListLimit, offset, limit, errMsg)
	}
}

func TestResolvePaging_Valid(t *testing.T) {
	cases := []struct {
		name       string
		offsetRaw  string
		limitRaw   string
		wantOffset int
		wantLimit  int
	}{
		{"常规翻页", "20", "20", 20, 20},
		{"首屏", "0", "50", 0, 50},
		{"上限 500（服务树绑定弹窗）", "0", "500", 0, 500},
		{"首尾空格", " 10 ", " 30 ", 10, 30},
		{"limit 纯空格视为未传", "", "   ", 0, defaultListLimit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			offset, limit, errMsg := resolvePaging(c.offsetRaw, c.limitRaw)
			if errMsg != "" {
				t.Fatalf("不应报错，实际 %q", errMsg)
			}
			if offset != c.wantOffset || limit != c.wantLimit {
				t.Fatalf("期望 (%d,%d)，实际 (%d,%d)", c.wantOffset, c.wantLimit, offset, limit)
			}
		})
	}
}

// 这些值改造前都会退化成「不限条数」，一次返回全量实例；
// 现在必须显式拒绝，避免资源放大与「以为分了页」的语义错误。
func TestResolvePaging_RejectsValuesThatUsedToDumpEverything(t *testing.T) {
	cases := []struct {
		name      string
		offsetRaw string
		limitRaw  string
		wantIn    string
	}{
		{"limit=0", "", "0", "limit 必须是 1 到 500"},
		{"limit=-1", "", "-1", "limit 必须是 1 到 500"},
		{"limit 非数字", "", "abc", "limit 必须是 1 到 500"},
		{"limit 超上限", "", "501", "limit 必须是 1 到 500"},
		{"limit 一百万", "", "1000000", "limit 必须是 1 到 500"},
		{"offset 负数", "-5", "20", "offset 必须是不小于 0 的整数"},
		{"offset 非数字", "x", "20", "offset 必须是不小于 0 的整数"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			offset, limit, errMsg := resolvePaging(c.offsetRaw, c.limitRaw)
			if errMsg == "" {
				t.Fatalf("应拒绝，实际返回 (%d,%d)", offset, limit)
			}
			if !contains(errMsg, c.wantIn) {
				t.Fatalf("错误信息应包含 %q，实际 %q", c.wantIn, errMsg)
			}
		})
	}
}

func TestMaxListLimitCoversFrontendUsage(t *testing.T) {
	// 前端最大取数场景：服务树绑定弹窗按 500 拉候选资源；调小上限会直接打断该页面。
	if maxListLimit < 500 {
		t.Fatalf("maxListLimit 必须 >= 500，实际 %d", maxListLimit)
	}
	// 模型关系页（/cmdb/relations）一次拉全 28 种关系类型，发的是 limit=1000。
	if maxModelRelationListLimit < 1000 {
		t.Fatalf("maxModelRelationListLimit 必须 >= 1000，实际 %d", maxModelRelationListLimit)
	}
}

// 不同列表端点的历史默认页大小不同，但校验语义必须一致：
// 非法值一律拒绝，合法值原样通过。
func TestResolvePagingWithin_CustomBounds(t *testing.T) {
	cases := []struct {
		name        string
		offsetRaw   string
		limitRaw    string
		defaultSize int
		maxSize     int
		wantOffset  int
		wantLimit   int
		wantErrIn   string
	}{
		{"属性列表缺省 100", "", "", attributeListLimit, maxListLimit, 0, 100, ""},
		{"属性列表前端取数 200", "0", "200", attributeListLimit, maxListLimit, 0, 200, ""},
		{"属性列表超上限", "", "501", attributeListLimit, maxListLimit, 0, 0, "limit 必须是 1 到 500"},
		{"模型关系一次拉全 1000", "0", "1000", defaultListLimit, maxModelRelationListLimit, 0, 1000, ""},
		{"模型关系超上限", "0", "1001", defaultListLimit, maxModelRelationListLimit, 0, 0, "limit 必须是 1 到 1000"},
		{"模型关系 limit=0 仍拒绝", "0", "0", defaultListLimit, maxModelRelationListLimit, 0, 0, "limit 必须是 1 到 1000"},
		{"自定义上限下负 offset 拒绝", "-1", "20", defaultListLimit, maxModelRelationListLimit, 0, 0, "offset 必须是不小于 0 的整数"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			offset, limit, errMsg := resolvePagingWithin(c.offsetRaw, c.limitRaw, c.defaultSize, c.maxSize)
			if c.wantErrIn != "" {
				if errMsg == "" {
					t.Fatalf("应拒绝，实际返回 (%d,%d)", offset, limit)
				}
				if !contains(errMsg, c.wantErrIn) {
					t.Fatalf("错误信息应包含 %q，实际 %q", c.wantErrIn, errMsg)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("不应报错，实际 %q", errMsg)
			}
			if offset != c.wantOffset || limit != c.wantLimit {
				t.Fatalf("期望 (%d,%d)，实际 (%d,%d)", c.wantOffset, c.wantLimit, offset, limit)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
