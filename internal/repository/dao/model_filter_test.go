package dao

import (
	"testing"
)

// TestModelBuildQuery_ModelGroupID 锁定模型分组过滤条件。
//
// 背景：handler 曾漏解析 `model_group_id` 查询参数，导致 filter.ModelGroupID
// 恒为 0、本分支永不触发，传任意分组都返回全部模型（实测 9 个分组全部返回 80 条）。
// handler 侧已修；此测试锁定 DAO 侧行为，防止另一端再被改坏。
func TestModelBuildQuery_ModelGroupID(t *testing.T) {
	d := &modelDAO{}

	t.Run("分组ID>0 时加入过滤条件", func(t *testing.T) {
		q := d.buildQuery(ModelFilter{ModelGroupID: 3})
		got, ok := q["model_group_id"]
		if !ok {
			t.Fatal("expected model_group_id in query")
		}
		if got != int64(3) {
			t.Fatalf("model_group_id = %v, want 3", got)
		}
	})

	t.Run("分组ID为0 时不加过滤（返回全部）", func(t *testing.T) {
		q := d.buildQuery(ModelFilter{ModelGroupID: 0})
		if _, ok := q["model_group_id"]; ok {
			t.Fatal("model_group_id should be absent when 0")
		}
	})

	t.Run("与其他条件可共存", func(t *testing.T) {
		q := d.buildQuery(ModelFilter{
			Provider:     "aliyun",
			Category:     "compute",
			ModelGroupID: 4,
		})
		if q["provider"] != "aliyun" {
			t.Fatalf("provider = %v", q["provider"])
		}
		if q["category"] != "compute" {
			t.Fatalf("category = %v", q["category"])
		}
		if q["model_group_id"] != int64(4) {
			t.Fatalf("model_group_id = %v", q["model_group_id"])
		}
	})

	t.Run("空过滤产生空条件", func(t *testing.T) {
		q := d.buildQuery(ModelFilter{})
		if len(q) != 0 {
			t.Fatalf("expected empty query, got %v", q)
		}
	})
}
