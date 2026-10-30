package service

import (
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// TestBuildOrphanSourceResult 校验孤儿来源聚合的组装逻辑：
// - Total 必须是各分组之和（而不是另外查一次 count，避免两次数不一致）
// - 顺序必须保持 DAO 的降序（前端直接按序展示 TopN）
// - 空输入必须返回空切片而非 nil（前端 v-for 友好）
func TestBuildOrphanSourceResult(t *testing.T) {
	rows := []repository.OrphanModelGroup{
		{ModelUID: "volcengine_disk", Count: 1783},
		{ModelUID: "volcengine_snapshot", Count: 1491},
		{ModelUID: "aliyun_disk", Count: 1314},
	}

	got := buildOrphanSourceResult(rows)

	if len(got.Groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(got.Groups))
	}
	wantTotal := int64(1783 + 1491 + 1314)
	if got.Total != wantTotal {
		t.Fatalf("total = %d, want %d", got.Total, wantTotal)
	}
	if got.Groups[0].ModelUID != "volcengine_disk" || got.Groups[0].Count != 1783 {
		t.Fatalf("first group = %#v, want volcengine_disk/1783", got.Groups[0])
	}
	if got.Groups[2].ModelUID != "aliyun_disk" {
		t.Fatalf("order not preserved: last = %s", got.Groups[2].ModelUID)
	}
}

// TestBuildOrphanSourceResult_Empty 空结果必须是长度为 0 的切片（非 nil），
// 且 Total 为 0 —— 前端据此显示空态而不是渲染 undefined。
func TestBuildOrphanSourceResult_Empty(t *testing.T) {
	got := buildOrphanSourceResult(nil)

	if got.Groups == nil {
		t.Fatal("groups is nil, want empty slice")
	}
	if len(got.Groups) != 0 {
		t.Fatalf("groups = %d, want 0", len(got.Groups))
	}
	if got.Total != 0 {
		t.Fatalf("total = %d, want 0", got.Total)
	}
}

// TestBuildOrphanSourceResult_EmptyModelUID 兜底：model_uid 缺失的孤儿
// （$exists:false 或空字符串）也计入总数，不能被丢弃，
// 否则 Total 会小于 overview 的 orphan_count。
func TestBuildOrphanSourceResult_EmptyModelUID(t *testing.T) {
	rows := []repository.OrphanModelGroup{
		{ModelUID: "", Count: 7},
		{ModelUID: "aliyun_disk", Count: 3},
	}

	got := buildOrphanSourceResult(rows)

	if got.Total != 10 {
		t.Fatalf("total = %d, want 10 (空 model_uid 也必须计入)", got.Total)
	}
	if len(got.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(got.Groups))
	}
}
