package web

import "testing"

// TestResolveInstanceSearchTerm 锁定「keyword 是 asset_name 的兼容别名」。
//
// 背景（实测 2026-11-17）：验收脚本用 keyword=zz-xxx- 过滤残留数据后逐条删除，
// 但实例列表接口从未读取 keyword —— 请求被静默降级为全量列表，
// 脚本因此删掉了 49 条真实实例。同类陷阱此前还出现过一次
// （asset_id 未被读取，误删 10 条）。本测试保证：只要调用方用了搜索框的叫法，
// 关键字就必须真正进入查询条件。
func TestResolveInstanceSearchTerm(t *testing.T) {
	cases := []struct {
		name      string
		assetName string
		keyword   string
		want      string
	}{
		{
			name:      "只传 asset_name 时原样使用",
			assetName: "i-abc123",
			want:      "i-abc123",
		},
		{
			name:    "只传 keyword 时作为别名生效",
			keyword: "zz-p223-",
			want:    "zz-p223-",
		},
		{
			name:      "两者同时传入时以 asset_name 为准",
			assetName: "web-01",
			keyword:   "ignored",
			want:      "web-01",
		},
		{
			name:    "keyword 前后空格被裁剪",
			keyword: "  zz-p223-  ",
			want:    "zz-p223-",
		},
		{
			name:      "asset_name 为纯空格时退回 keyword",
			assetName: "   ",
			keyword:   "snap-1",
			want:      "snap-1",
		},
		{
			name: "两者都为空时是空关键字（全量列表）",
			want: "",
		},
		{
			name:      "asset_name 内部空格保持字面量",
			assetName: "web 01",
			want:      "web 01",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveInstanceSearchTerm(c.assetName, c.keyword); got != c.want {
				t.Fatalf("resolveInstanceSearchTerm(%q, %q) = %q, want %q",
					c.assetName, c.keyword, got, c.want)
			}
		})
	}
}
