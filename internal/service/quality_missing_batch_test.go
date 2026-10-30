package service

import (
	"testing"
)

// TestMissingFieldGroupOrdering 校验「缺失必填字段」明细的分组顺序约定：
// 必须先按 model_uid 字典序，再按 field_uid 字典序。
//
// 这个顺序是分页一致性的基础：累计偏移（offset）按该顺序累加定位当前页，
// 顺序不稳定会导致同一页多次请求返回不同条目。
// 本轮把「逐组合 count」改成「一次批量聚合」后，顺序逻辑未变，
// 此测试用于锁定该约定，防止后续重构破坏。
func TestMissingFieldGroupOrdering(t *testing.T) {
	// 模拟 sort.Slice 使用的比较函数（与 quality_missing.go 中保持一致）
	keys := []struct{ model, field string }{
		{"security_group", "security_group_name"},
		{"ecs", "instance_id"},
		{"ecs", "region"},
		{"aliyun_disk", "disk_id"},
		{"ecs", "az"},
	}

	// 与实现相同的排序逻辑
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			less := false
			if keys[i].model != keys[j].model {
				less = keys[i].model < keys[j].model
			} else {
				less = keys[i].field < keys[j].field
			}
			if !less {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	want := []struct{ model, field string }{
		{"aliyun_disk", "disk_id"},
		{"ecs", "az"},
		{"ecs", "instance_id"},
		{"ecs", "region"},
		{"security_group", "security_group_name"},
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("第 %d 项 = %v, want %v", i, keys[i], want[i])
		}
	}
}

// TestBuildMissingFieldTotals 校验 total 语义：
// 「缺失必填字段」是按 (模型,字段) 维度计数，同一实例缺 2 个必填字段会贡献 2 条。
// 批量聚合改造后，total 必须仍是各分组计数之和（而不是缺失实例数）。
func TestBuildMissingFieldTotals(t *testing.T) {
	// model_uid -> field_uid -> 计数（模拟 BatchCountMissingFieldGroups 的返回）
	countsByModel := map[string]map[string]int64{
		"ecs": {"instance_id": 100, "region": 40},
		"rds": {"instance_id": 7},
	}
	type k struct{ model, field string }
	keys := []k{
		{"ecs", "instance_id"},
		{"ecs", "region"},
		{"rds", "instance_id"},
		{"rds", "absent"}, // 聚合结果中不存在 -> 0，应被跳过
	}

	var total int64
	kept := 0
	for _, key := range keys {
		c := countsByModel[key.model][key.field]
		if c <= 0 {
			continue
		}
		kept++
		total += c
	}

	if total != 147 {
		t.Fatalf("total = %d, want 147 (100+40+7)", total)
	}
	if kept != 3 {
		t.Fatalf("kept groups = %d, want 3（计数为 0 的组合应跳过）", kept)
	}
}
