package dao

import (
	"testing"
)

// TestBuildQuery_AssetIDExactMatch 锁定「asset_id 精确匹配」过滤条件。
//
// 背景（实测 2026-11-01）：实例列表接口此前完全不读取 asset_id 查询参数，
// 传入会被静默忽略并返回全量数据。调用方按 asset_id 定位单条记录后再执行删除时，
// 实际作用于全量列表的前 N 条，造成 10 条真实实例被误删。
//
// 本测试确保该参数一旦被传入，就必须真正参与查询条件，防止两端任一侧被改坏。
func TestBuildQuery_AssetIDExactMatch(t *testing.T) {
	d := &instanceDAO{}

	t.Run("传入 asset_id 时应生成精确匹配条件", func(t *testing.T) {
		q := d.buildQuery(InstanceFilter{AssetID: "i-bp1234567890"})
		got, ok := q["asset_id"]
		if !ok {
			t.Fatal("expected asset_id in query：筛选失效会放大后续操作的影响范围")
		}
		if got != "i-bp1234567890" {
			t.Fatalf("asset_id = %v, want i-bp1234567890", got)
		}
	})

	t.Run("未传 asset_id 时不应产生该条件", func(t *testing.T) {
		q := d.buildQuery(InstanceFilter{})
		if _, ok := q["asset_id"]; ok {
			t.Fatal("asset_id should be absent when empty")
		}
	})

	t.Run("asset_id 与 tenant_id 应 AND 叠加", func(t *testing.T) {
		q := d.buildQuery(InstanceFilter{AssetID: "i-abc", TenantID: 3})
		if q["asset_id"] != "i-abc" {
			t.Fatalf("asset_id = %v, want i-abc", q["asset_id"])
		}
		if q["tenant_id"] != int64(3) {
			t.Fatalf("tenant_id = %v, want 3：租户条件必须同时存在", q["tenant_id"])
		}
	})

	t.Run("asset_id 与 asset_name 可共存", func(t *testing.T) {
		q := d.buildQuery(InstanceFilter{AssetID: "i-abc", AssetName: "web"})
		if q["asset_id"] != "i-abc" {
			t.Fatalf("asset_id = %v, want i-abc", q["asset_id"])
		}
		if _, ok := q["$or"]; !ok {
			t.Fatal("asset_name 仍应生成 $or 模糊条件")
		}
	})

	t.Run("asset_id 应做字面量精确匹配而非正则", func(t *testing.T) {
		q := d.buildQuery(InstanceFilter{AssetID: "i.abc*"})
		got, ok := q["asset_id"].(string)
		if !ok {
			t.Fatalf("asset_id 应为 string 精确值，实际 %T", q["asset_id"])
		}
		if got != "i.abc*" {
			t.Fatalf("asset_id = %q, want 字面量 i.abc*", got)
		}
	})
}
