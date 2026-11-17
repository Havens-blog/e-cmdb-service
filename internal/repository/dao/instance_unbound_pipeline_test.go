package dao

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestUnboundPipeline 回归（实测 2026-11-17）：
// 1) 联表集合必须用 ecam_resource_binding（绑定实际落点），不能用遗留 c_resource_binding，
// 否则已绑定资产全被当作未绑定（unbound=实例总数）；
// 2) resource_type 既有 rule 写入的 instance 也有 manual 写入的 asset，必须同时匹配。
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
	sub, ok := lookup["pipeline"].(mongo.Pipeline)
	if !ok {
		t.Fatalf("lookup.pipeline 类型错误: %T", lookup["pipeline"])
	}
	got := map[string]bool{}
	collectStrings(sub, got)
	if !got["instance"] || !got["asset"] {
		t.Fatalf("resource_type 应同时匹配 instance/asset，实际: %v", got)
	}
}

func collectStrings(v interface{}, out map[string]bool) {
	switch x := v.(type) {
	case string:
		out[x] = true
	case bson.D:
		for _, e := range x {
			collectStrings(e.Value, out)
		}
	case bson.M:
		for k, vv := range x {
			out[k] = true
			collectStrings(vv, out)
		}
	case bson.A:
		for _, item := range x {
			collectStrings(item, out)
		}
	case mongo.Pipeline:
		for _, dd := range x {
			collectStrings(dd, out)
		}
	}
}