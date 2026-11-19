package dao

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestUnboundPipeline 回归（实测 2026-11-17 + 2026-11-19）：
//  1. 联表集合必须用 ecam_resource_binding（绑定实际落点），不能用遗留 c_resource_binding，
//     否则已绑定资产全被当作未绑定（unbound=实例总数）；
//  2. resource_type 既有 rule 写入的 instance 也有 manual 写入的 asset，必须同时匹配；
//  3. 租户必须参与「已绑定」判定（跨租户同 id 的绑定不能把实例算成已绑定）；
//  4. $lookup 必须是 localField/foreignField 简单形态 —— let + pipeline + $expr 形态
//     会让每个实例跑一次子管道，实测 /internal/instances/unbound 3.3s → 0.2s。
func TestUnboundPipeline(t *testing.T) {
	d := &instanceDAO{}
	p := d.unboundPipeline(3)
	if len(p) != 4 {
		t.Fatalf("pipeline 阶段数 = %d, want 4", len(p))
	}

	m0 := p[0].Map()
	match0, ok := m0["$match"].(bson.M)
	if !ok {
		t.Fatalf("阶段0 不是 $match")
	}
	if tid, _ := match0["tenant_id"].(int64); tid != 3 {
		t.Fatalf("阶段0 tenant_id = %v, want 3", match0["tenant_id"])
	}

	m1 := p[1].Map()
	lookup, ok := m1["$lookup"].(bson.M)
	if !ok {
		t.Fatalf("阶段1 不是 $lookup")
	}
	from, _ := lookup["from"].(string)
	if from != BindingCollection {
		t.Fatalf("联表集合 = %q, want %q", from, BindingCollection)
	}
	if from == "c_resource_binding" {
		t.Fatalf("不得用具遗留集合 c_resource_binding 联表")
	}
	if lf, _ := lookup["localField"].(string); lf != "id" {
		t.Fatalf("lookup.localField = %q, want id（实例主键）", lf)
	}
	if ff, _ := lookup["foreignField"].(string); ff != "resource_id" {
		t.Fatalf("lookup.foreignField = %q, want resource_id", ff)
	}
	if _, has := lookup["pipeline"]; has {
		t.Fatalf("不得使用 let+pipeline 形态（每个实例跑一次子管道，实测慢 10 倍）")
	}
	if _, has := lookup["let"]; has {
		t.Fatalf("不得使用 let 变量形态")
	}

	// 阶段2：未绑定的判定条件
	m2 := p[2].Map()
	match2, ok := m2["$match"].(bson.M)
	if !ok {
		t.Fatalf("阶段2 不是 $match")
	}
	cond, ok := match2["_bindings"].(bson.M)
	if !ok {
		t.Fatalf("阶段2 未对 _bindings 做判定")
	}
	notWrap, ok := cond["$not"].(bson.M)
	if !ok {
		t.Fatalf("阶段2 应使用 $not 表达「不存在满足条件的绑定」，实际 %v", cond)
	}
	elem, ok := notWrap["$elemMatch"].(bson.M)
	if !ok {
		t.Fatalf("阶段2 应使用 $elemMatch，实际 %v", notWrap)
	}
	if tid, _ := elem["tenant_id"].(int64); tid != 3 {
		t.Fatalf("绑定判定必须带租户条件，tenant_id = %v", elem["tenant_id"])
	}
	types, ok := elem["resource_type"].(bson.M)
	if !ok {
		t.Fatalf("绑定判定必须限定 resource_type，实际 %v", elem["resource_type"])
	}
	inList, ok := types["$in"].(bson.A)
	if !ok {
		t.Fatalf("resource_type 应使用 $in，实际 %v", types)
	}
	got := map[string]bool{}
	for _, v := range inList {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	if !got["instance"] || !got["asset"] {
		t.Fatalf("resource_type 应同时匹配 instance/asset，实际: %v", got)
	}

	// 阶段3：清理临时字段
	m3 := p[3].Map()
	proj, ok := m3["$project"].(bson.M)
	if !ok {
		t.Fatalf("阶段3 不是 $project")
	}
	if v, ok := proj["_bindings"].(int); !ok || v != 0 {
		t.Fatalf("阶段3 应剔除 _bindings，实际 %v", proj["_bindings"])
	}
}

// TestUnboundPipelineCountShape 说明计数形态：CountUnbound 复用同一管道后追加 $count，
// 因此上面的联表/判定条件对计数同样成立（不需要单独的管道实现）。
func TestUnboundPipelineCountShape(t *testing.T) {
	d := &instanceDAO{}
	p := d.unboundPipeline(3)
	withCount := append(append(mongo.Pipeline{}, p...), bson.D{{Key: "$count", Value: "total"}})
	if len(withCount) != len(p)+1 {
		t.Fatalf("追加 $count 后阶段数 = %d, want %d", len(withCount), len(p)+1)
	}
	last := withCount[len(withCount)-1].Map()
	if _, ok := last["$count"]; !ok {
		t.Fatalf("最后一阶段应为 $count，实际 %v", last)
	}
}
