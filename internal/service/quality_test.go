package service

import (
	"testing"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

func TestAnalyzeQuality_Empty(t *testing.T) {
	a := analyzeQuality(nil, nil, nil, nil)

	if a.totalInstances != 0 {
		t.Fatalf("total_instances = %d, want 0", a.totalInstances)
	}
	if a.orphanCount != 0 || a.missingFieldCount != 0 || a.danglingRelationCount != 0 {
		t.Fatalf("unexpected counts: orphan=%d missing=%d dangling=%d",
			a.orphanCount, a.missingFieldCount, a.danglingRelationCount)
	}
	if a.healthScore != 100.0 {
		t.Fatalf("health_score = %v, want 100", a.healthScore)
	}
	if len(a.issues) != 0 {
		t.Fatalf("issues = %#v, want empty", a.issues)
	}
}

func TestAnalyzeQuality_Orphan(t *testing.T) {
	models := []domain.Model{{UID: "m1"}}
	instances := []domain.Instance{
		{ID: 1, ModelUID: "missing_model", AssetID: "a1", AssetName: "n1"},
	}

	a := analyzeQuality(models, nil, instances, nil)

	if a.totalInstances != 1 {
		t.Fatalf("total_instances = %d, want 1", a.totalInstances)
	}
	if a.orphanCount != 1 {
		t.Fatalf("orphan_count = %d, want 1", a.orphanCount)
	}
	if a.healthScore != 0.0 {
		t.Fatalf("health_score = %v, want 0", a.healthScore)
	}
	if len(a.issues) != 1 || a.issues[0].Type != QualityIssueTypeOrphan {
		t.Fatalf("issues = %#v, want single orphan", a.issues)
	}
	if a.issues[0].InstanceID != 1 || a.issues[0].ModelUID != "missing_model" {
		t.Fatalf("orphan issue fields wrong: %#v", a.issues[0])
	}
}

func TestAnalyzeQuality_MissingRequiredField(t *testing.T) {
	models := []domain.Model{{UID: "m1"}}
	attributes := []domain.Attribute{
		{ModelUID: "m1", FieldUID: "cpu", FieldName: "CPU", Required: true},
		{ModelUID: "m1", FieldUID: "memory", FieldName: "内存", Required: false},
	}
	instances := []domain.Instance{
		{ID: 1, ModelUID: "m1", AssetID: "a1", AssetName: "n1", Attributes: map[string]interface{}{}},
	}

	a := analyzeQuality(models, attributes, instances, nil)

	if a.missingFieldCount != 1 {
		t.Fatalf("missing_field_count = %d, want 1", a.missingFieldCount)
	}
	if len(a.issues) != 1 || a.issues[0].Type != QualityIssueTypeMissingField {
		t.Fatalf("issues = %#v, want single missing_field", a.issues)
	}
	if a.issues[0].FieldUID != "cpu" || a.issues[0].FieldName != "CPU" {
		t.Fatalf("missing field issue fields wrong: %#v", a.issues[0])
	}
}

func TestAnalyzeQuality_MissingAssetIDAndModelUID(t *testing.T) {
	models := []domain.Model{{UID: "m1"}}
	instances := []domain.Instance{
		{ID: 1, ModelUID: "m1", AssetID: "", AssetName: "n1"},
		{ID: 2, ModelUID: "", AssetID: "a2", AssetName: "n2"},
	}

	a := analyzeQuality(models, nil, instances, nil)

	// 实例1：缺失 asset_id；实例2：orphan + 缺失 model_uid。
	if a.orphanCount != 1 {
		t.Fatalf("orphan_count = %d, want 1", a.orphanCount)
	}
	if a.missingFieldCount != 2 {
		t.Fatalf("missing_field_count = %d, want 2", a.missingFieldCount)
	}

	fieldUIDs := map[string]bool{}
	for _, issue := range a.issues {
		if issue.Type != QualityIssueTypeMissingField {
			continue
		}
		fieldUIDs[issue.FieldUID] = true
	}
	if !fieldUIDs["asset_id"] || !fieldUIDs["model_uid"] {
		t.Fatalf("missing_field issues should include asset_id and model_uid: %#v", a.issues)
	}
}

func TestAnalyzeQuality_DanglingRelation(t *testing.T) {
	models := []domain.Model{{UID: "m1"}}
	instances := []domain.Instance{
		{ID: 1, ModelUID: "m1", AssetID: "a1"},
		{ID: 2, ModelUID: "m1", AssetID: "a2"},
	}
	relations := []domain.InstanceRelation{
		{ID: 10, SourceInstanceID: 1, TargetInstanceID: 99},
		{ID: 11, SourceInstanceID: 88, TargetInstanceID: 2},
		{ID: 12, SourceInstanceID: 1, TargetInstanceID: 2},
	}

	a := analyzeQuality(models, nil, instances, relations)

	if a.danglingRelationCount != 2 {
		t.Fatalf("dangling_relation_count = %d, want 2", a.danglingRelationCount)
	}
	if len(a.issues) != 2 {
		t.Fatalf("issues = %#v, want 2 dangling", a.issues)
	}
	for _, issue := range a.issues {
		if issue.Type != QualityIssueTypeDanglingRelation {
			t.Fatalf("unexpected issue type: %#v", issue)
		}
	}
}

func TestComputeHealthScore(t *testing.T) {
	cases := []struct {
		name   string
		total  int
		issues int
		want   float64
	}{
		{"empty", 0, 0, 100.0},
		{"two of ten", 10, 2, 80.0},
		{"round one decimal", 3, 1, 66.7},
		{"clamp low", 2, 5, 0.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeHealthScore(tc.total, tc.issues); got != tc.want {
				t.Fatalf("computeHealthScore(%d, %d) = %v, want %v", tc.total, tc.issues, got, tc.want)
			}
		})
	}
}

func TestSelectIssues_FilterAndPagination(t *testing.T) {
	issues := []QualityIssue{
		{Type: QualityIssueTypeOrphan, InstanceID: 1},
		{Type: QualityIssueTypeMissingField, InstanceID: 1, FieldUID: "cpu"},
		{Type: QualityIssueTypeOrphan, InstanceID: 2},
		{Type: QualityIssueTypeOrphan, InstanceID: 3},
	}

	got := selectIssues(issues, QualityIssueTypeOrphan, 0, 2)
	if got.Total != 3 {
		t.Fatalf("total = %d, want 3", got.Total)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(got.Items))
	}
	if got.Items[0].InstanceID != 1 || got.Items[1].InstanceID != 2 {
		t.Fatalf("unexpected page items: %#v", got.Items)
	}

	got = selectIssues(issues, QualityIssueTypeOrphan, 2, 10)
	if got.Total != 3 || len(got.Items) != 1 || got.Items[0].InstanceID != 3 {
		t.Fatalf("unexpected last page: %#v", got)
	}

	got = selectIssues(issues, QualityIssueTypeOrphan, 99, 10)
	if got.Total != 3 || len(got.Items) != 0 {
		t.Fatalf("unexpected out-of-range page: %#v", got)
	}

	got = selectIssues(issues, "unknown", 0, 10)
	if got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("unknown type should be empty: %#v", got)
	}
}

// TestAnalysisCache_HitAndExpire 验证质量分析缓存的基本语义：
// 未写入时 miss、写入后命中、过期后再次 miss、不同租户互不影响。
func TestAnalysisCache_HitAndExpire(t *testing.T) {
	c := newAnalysisCache()

	if _, ok := c.get(3); ok {
		t.Fatal("empty cache should miss")
	}

	want := qualityAnalysis{totalInstances: 42, orphanCount: 7}
	c.put(3, want)

	got, ok := c.get(3)
	if !ok {
		t.Fatal("expected cache hit after put")
	}
	if got.totalInstances != 42 || got.orphanCount != 7 {
		t.Fatalf("cached value = %+v, want total=42 orphan=7", got)
	}

	// 不同租户不应命中
	if _, ok := c.get(4); ok {
		t.Fatal("different tenant should miss")
	}

	// 手动把过期时间拨到过去，验证过期后 miss
	c.mu.Lock()
	e := c.entries[3]
	e.expireAt = time.Now().Add(-time.Second)
	c.entries[3] = e
	c.mu.Unlock()

	if _, ok := c.get(3); ok {
		t.Fatal("expired entry should miss")
	}
}
