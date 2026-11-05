package dao

import (
	"testing"
)

// TestBuildQuery_SourceOwner 锁定实例列表「来源」「责任人」筛选的查询构造：
// 设置时按等值进入查询，未设置时不产生条件。
//
// 背景（2026-11-05）：source/owner 字段（2026-11-04 加字段）此前只能展示不能筛选，
// 本测试锁定筛选闭环的 DAO 层接线。33k 量级无索引全表扫可接受，量级增长后再议索引。
func TestBuildQuery_SourceOwner(t *testing.T) {
	t.Run("设置 source 与 owner", func(t *testing.T) {
		d := &instanceDAO{}
		q := d.buildQuery(InstanceFilter{
			TenantID: 3,
			Source:   "manual",
			Owner:    "zhangsan",
		})
		if q["source"] != "manual" {
			t.Fatalf("source 条件错误: %v", q["source"])
		}
		if q["owner"] != "zhangsan" {
			t.Fatalf("owner 条件错误: %v", q["owner"])
		}
		if q["tenant_id"] != int64(3) {
			t.Fatalf("tenant_id 条件错误: %v", q["tenant_id"])
		}
	})

	t.Run("未设置时不产生条件", func(t *testing.T) {
		d := &instanceDAO{}
		q := d.buildQuery(InstanceFilter{TenantID: 3})
		if _, ok := q["source"]; ok {
			t.Fatalf("未设置 source 不应产生条件: %v", q)
		}
		if _, ok := q["owner"]; ok {
			t.Fatalf("未设置 owner 不应产生条件: %v", q)
		}
	})
}
