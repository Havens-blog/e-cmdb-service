package dao

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestBuildFacetPipeline 锁定实例筛选可选值的聚合管道形状。
//
// 背景（2026-11-18 实测）：前端把「云平台」硬编码成 5 项且火山引擎写成 volcengine，
// 而库里 provider=volcano 有 7,813 条 → 选中「火山引擎」筛出 0 条；「区域」下拉
// 由当前页 20 条提取，翻页后选项随之变化。改为服务端 $facet 聚合后，这里锁定：
//   - 必须按租户过滤（筛选可选值也不能跨租户）
//   - provider/region/source/status 四个维度都要分桶，缺失值经 $ifNull 归一为空串
//   - 每个维度要有上限，避免属性爆炸时把响应撑大
func TestBuildFacetPipeline(t *testing.T) {
	pipeline := buildFacetPipeline(3, 100)
	if len(pipeline) != 2 {
		t.Fatalf("管道应有两段（$match + $facet），实际 %d", len(pipeline))
	}

	// 第一段：租户过滤
	matchStage := pipeline[0]
	if len(matchStage) != 1 || matchStage[0].Key != "$match" {
		t.Fatalf("首段必须是 $match: %v", matchStage)
	}
	match, ok := matchStage[0].Value.(bson.M)
	if !ok {
		t.Fatalf("$match 值类型应为 bson.M，实际 %T", matchStage[0].Value)
	}
	if got := match["tenant_id"]; got != int64(3) {
		t.Fatalf("租户谓词缺失或错误: %v", got)
	}

	// 第二段：四个 facet 分支
	facetStage := pipeline[1]
	if len(facetStage) != 1 || facetStage[0].Key != "$facet" {
		t.Fatalf("次段必须是 $facet: %v", facetStage)
	}
	spec, ok := facetStage[0].Value.(bson.D)
	if !ok {
		t.Fatalf("$facet 值类型应为 bson.D，实际 %T", facetStage[0].Value)
	}
	branches := make(map[string]bson.A, len(spec))
	for _, item := range spec {
		if arr, ok := item.Value.(bson.A); ok {
			branches[item.Key] = arr
		}
	}
	for _, key := range []string{"providers", "regions", "sources", "statuses", "total"} {
		if _, ok := branches[key]; !ok {
			t.Fatalf("$facet 缺少维度 %q", key)
		}
	}

	cases := map[string]string{
		"providers": "attributes.provider",
		"regions":   "attributes.region",
		"sources":   "source",
		// iter246：状态筛选。状态值是小写且按模型不同（running/in_use/in-use/attached…），
		// 只能由服务端聚合给出，硬编码大写会筛出 0 条。
		"statuses": "attributes.status",
	}
	for branch, field := range cases {
		stages := branchStages(t, branch, branches[branch])
		group := stages["$group"]
		if group == nil {
			t.Fatalf("%s 分支缺少 $group", branch)
		}
		if !containsFieldRef(group, "$"+field) {
			t.Fatalf("%s 分支未按 %s 分组: %v", branch, field, group)
		}
		if !containsIfNull(group) {
			t.Fatalf("%s 分支必须用 $ifNull 归一缺失值（否则 null 无法解码为 string）: %v", branch, group)
		}
		if stages["$limit"] == nil {
			t.Fatalf("%s 分支缺少 $limit（防止取值爆炸）", branch)
		}
	}

	// total 分支用 $count，不应带 $group
	total := branchStages(t, "total", branches["total"])
	if total["$count"] == nil {
		t.Fatalf("total 分支必须是 $count: %v", total)
	}
}

// TestBuildFacetPipeline_TenantIsolation 不同租户的匹配条件必须不同，
// 防止复制粘贴时丢掉租户谓词。
func TestBuildFacetPipeline_TenantIsolation(t *testing.T) {
	a := buildFacetPipeline(1, 100)[0][0].Value.(bson.M)["tenant_id"]
	b := buildFacetPipeline(3, 100)[0][0].Value.(bson.M)["tenant_id"]
	if a == b {
		t.Fatalf("不同租户的匹配条件相同，租户谓词被丢弃: %v", a)
	}
}

// TestBuildFacetPipeline_LimitApplied 上限参数必须真正进入管道。
func TestBuildFacetPipeline_LimitApplied(t *testing.T) {
	pipeline := buildFacetPipeline(3, 7)
	spec := pipeline[1][0].Value.(bson.D)
	for _, item := range spec {
		if item.Key != "providers" {
			continue
		}
		arr := item.Value.(bson.A)
		last := arr[len(arr)-1].(bson.D)
		if last[0].Key != "$limit" {
			t.Fatalf("providers 分支最后一段应为 $limit: %v", last)
		}
		if last[0].Value != 7 {
			t.Fatalf("$limit 未使用传入上限: %v", last[0].Value)
		}
		return
	}
	t.Fatalf("未找到 providers 分支")
}

// branchStages 把分支的 bson.A 转成「阶段名 → 值」。
func branchStages(t *testing.T, name string, arr bson.A) map[string]interface{} {
	t.Helper()
	out := make(map[string]interface{}, len(arr))
	for _, stage := range arr {
		doc, ok := stage.(bson.D)
		if !ok || len(doc) != 1 {
			t.Fatalf("%s 分支阶段格式异常: %T %v", name, stage, stage)
		}
		out[doc[0].Key] = doc[0].Value
	}
	return out
}

// containsFieldRef 递归查找分组键里的字段引用。
func containsFieldRef(v interface{}, want string) bool {
	switch x := v.(type) {
	case string:
		return x == want
	case bson.D:
		for _, item := range x {
			if containsFieldRef(item.Key, want) || containsFieldRef(item.Value, want) {
				return true
			}
		}
	case bson.M:
		for k, val := range x {
			if containsFieldRef(k, want) || containsFieldRef(val, want) {
				return true
			}
		}
	case bson.A:
		for _, item := range x {
			if containsFieldRef(item, want) {
				return true
			}
		}
	}
	return false
}

// containsIfNull 判断分组键里是否用了 $ifNull。
func containsIfNull(v interface{}) bool {
	return containsFieldRef(v, "$ifNull")
}
