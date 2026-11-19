package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// 第238轮：字段值不合规（类型/枚举）清单的产出与筛选分页。
//
// 背景：方案 B 下枚举越界/类型不符只警告不阻断写入，而记录入口
// sync_quality_issue 只被 e-cmdb 自身 upsert 调用，真实云同步由 e-cam
// 直接写库 —— 这类脏数据在系统里完全不可见。collectFieldValueIssues
// 负责把它们提升成可检索清单，语义必须与 domain.ValidateAttributes 一致。

func fieldIssueAttrs(modelUID string) []domain.Attribute {
	return []domain.Attribute{
		{ModelUID: modelUID, FieldUID: "cpu", FieldName: "cpu", DisplayName: "CPU 核数", FieldType: "int"},
		{
			ModelUID: modelUID, FieldUID: "status", FieldName: "status", DisplayName: "状态",
			FieldType: "enum", Option: []interface{}{"Running", "Stopped"},
		},
		{ModelUID: modelUID, FieldUID: "hostname", FieldName: "hostname", FieldType: "string", Required: true},
	}
}

func TestCollectFieldValueIssues(t *testing.T) {
	attrs := fieldIssueAttrs("ecs")

	t.Run("类型不符与枚举越界都被产出", func(t *testing.T) {
		instances := []domain.Instance{
			{
				ID: 11, AssetID: "zz-r238-type", AssetName: "类型不符", ModelUID: "ecs",
				Attributes: map[string]interface{}{"cpu": "not-a-number", "status": "Running", "hostname": "h1"},
			},
			{
				ID: 12, AssetID: "zz-r238-enum", AssetName: "枚举越界", ModelUID: "ecs",
				Attributes: map[string]interface{}{"cpu": float64(4), "status": "THIS_IS_NOT_A_STATUS", "hostname": "h2"},
			},
		}

		got := collectFieldValueIssues(instances, attrs)
		if len(got) != 2 {
			t.Fatalf("期望 2 条问题，实际 %d 条：%+v", len(got), got)
		}

		byType := map[string]QualityIssue{}
		for _, issue := range got {
			byType[issue.Type] = issue
		}

		typeIssue, ok := byType[QualityIssueTypeFieldType]
		if !ok {
			t.Fatalf("缺少类型不符问题：%+v", got)
		}
		if typeIssue.FieldUID != "cpu" || typeIssue.InstanceID != 11 || typeIssue.ModelUID != "ecs" {
			t.Errorf("类型不符问题定位错误：%+v", typeIssue)
		}
		if typeIssue.FieldName != "CPU 核数" {
			t.Errorf("应使用字段显示名，实际 %q", typeIssue.FieldName)
		}
		if typeIssue.AssetID != "zz-r238-type" {
			t.Errorf("应带资产ID，实际 %q", typeIssue.AssetID)
		}

		enumIssue, ok := byType[QualityIssueTypeFieldEnum]
		if !ok {
			t.Fatalf("缺少枚举越界问题：%+v", got)
		}
		if enumIssue.FieldUID != "status" || enumIssue.InstanceID != 12 {
			t.Errorf("枚举越界问题定位错误：%+v", enumIssue)
		}
		if enumIssue.FieldName != "状态" {
			t.Errorf("应使用字段显示名，实际 %q", enumIssue.FieldName)
		}
	})

	t.Run("合规实例不产出问题", func(t *testing.T) {
		instances := []domain.Instance{{
			ID: 21, AssetID: "zz-r238-ok", ModelUID: "ecs",
			Attributes: map[string]interface{}{"cpu": "8", "status": "running", "hostname": "h3"},
		}}
		if got := collectFieldValueIssues(instances, attrs); len(got) != 0 {
			t.Fatalf("合规实例不应有问题，实际 %+v", got)
		}
	})

	t.Run("必填缺失不进入字段值清单", func(t *testing.T) {
		// hostname 是必填但没填：属 missing_field 口径，不应重复计入字段值问题
		instances := []domain.Instance{{
			ID: 31, AssetID: "zz-r238-required", ModelUID: "ecs",
			Attributes: map[string]interface{}{"cpu": float64(2), "status": "Stopped"},
		}}
		if got := collectFieldValueIssues(instances, attrs); len(got) != 0 {
			t.Fatalf("必填缺失不应进入字段值清单，实际 %+v", got)
		}
	})

	t.Run("模型无字段定义时跳过", func(t *testing.T) {
		instances := []domain.Instance{{
			ID: 41, AssetID: "zz-r238-alias", ModelUID: "aliyun_ecs",
			Attributes: map[string]interface{}{"cpu": "garbage", "status": "garbage"},
		}}
		if got := collectFieldValueIssues(instances, attrs); len(got) != 0 {
			t.Fatalf("无定义模型不应产出问题，实际 %+v", got)
		}
	})

	t.Run("结果按模型字段实例稳定排序", func(t *testing.T) {
		instances := []domain.Instance{
			{ID: 13, AssetID: "a", ModelUID: "ecs", Attributes: map[string]interface{}{"status": "bad"}},
			{ID: 12, AssetID: "b", ModelUID: "ecs", Attributes: map[string]interface{}{"status": "bad"}},
		}
		got := collectFieldValueIssues(instances, attrs)
		if len(got) != 2 {
			t.Fatalf("期望 2 条，实际 %d", len(got))
		}
		if got[0].InstanceID != 12 || got[1].InstanceID != 13 {
			t.Fatalf("应按实例ID升序，实际 %d → %d", got[0].InstanceID, got[1].InstanceID)
		}
	})
}

func TestSelectFieldIssues(t *testing.T) {
	issues := []QualityIssue{
		{Type: QualityIssueTypeFieldType, ModelUID: "ecs", FieldUID: "cpu", InstanceID: 1},
		{Type: QualityIssueTypeFieldEnum, ModelUID: "ecs", FieldUID: "status", InstanceID: 2},
		{Type: QualityIssueTypeFieldEnum, ModelUID: "rds", FieldUID: "status", InstanceID: 3},
	}

	t.Run("不筛选时返回全部并分页", func(t *testing.T) {
		got := selectFieldIssues(issues, "", "", 0, 2)
		if got.Total != 3 || len(got.Items) != 2 {
			t.Fatalf("期望 total=3 首页2条，实际 total=%d len=%d", got.Total, len(got.Items))
		}
		got = selectFieldIssues(issues, "", "", 2, 2)
		if got.Total != 3 || len(got.Items) != 1 || got.Items[0].InstanceID != 3 {
			t.Fatalf("第二页应剩 1 条实例3，实际 %+v", got.Items)
		}
	})

	t.Run("按模型筛选", func(t *testing.T) {
		got := selectFieldIssues(issues, "ecs", "", 0, 20)
		if got.Total != 2 {
			t.Fatalf("ecs 应 2 条，实际 %d", got.Total)
		}
	})

	t.Run("按类型筛选", func(t *testing.T) {
		got := selectFieldIssues(issues, "", QualityIssueTypeFieldEnum, 0, 20)
		if got.Total != 2 {
			t.Fatalf("enum 应 2 条，实际 %d", got.Total)
		}
		for _, issue := range got.Items {
			if issue.Type != QualityIssueTypeFieldEnum {
				t.Fatalf("类型筛选失效：%+v", issue)
			}
		}
	})

	t.Run("模型与类型同时筛选", func(t *testing.T) {
		got := selectFieldIssues(issues, "rds", QualityIssueTypeFieldEnum, 0, 20)
		if got.Total != 1 || got.Items[0].InstanceID != 3 {
			t.Fatalf("rds+enum 应 1 条实例3，实际 %+v", got)
		}
	})

	t.Run("越界分页返回空列表但 total 不变", func(t *testing.T) {
		got := selectFieldIssues(issues, "", "", 99, 20)
		if got.Total != 3 || len(got.Items) != 0 {
			t.Fatalf("越界页应空列表 total=3，实际 total=%d len=%d", got.Total, len(got.Items))
		}
	})
}

// fieldIssueService 用最小假仓库跑通 FieldIssues 全路径（分析缓存 + 全量回退）。
type fieldIssueInstanceRepo struct {
	*fakeInstanceRepo
	instances []domain.Instance
}

func (f *fieldIssueInstanceRepo) List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, error) {
	return f.instances, nil
}

type fieldIssueModelRepo struct {
	*fakeModelRepo
	models []domain.Model
}

func (f *fieldIssueModelRepo) List(ctx context.Context, filter domain.ModelFilter) ([]domain.Model, error) {
	return f.models, nil
}

func newFieldIssueService(instances []domain.Instance, attrs []domain.Attribute) QualityService {
	models := make([]domain.Model, 0)
	seen := map[string]bool{}
	for _, attr := range attrs {
		if !seen[attr.ModelUID] {
			seen[attr.ModelUID] = true
			models = append(models, domain.Model{UID: attr.ModelUID, Name: attr.ModelUID})
		}
	}
	return NewQualityService(
		&fieldIssueInstanceRepo{fakeInstanceRepo: newFakeInstanceRepo(), instances: instances},
		&fieldIssueModelRepo{fakeModelRepo: &fakeModelRepo{}, models: models},
		newFakeRelationRepo(),
		&fakeAttributeRepoForSync{defs: attrs},
		nil,
	)
}

func TestFieldIssuesEndToEnd(t *testing.T) {
	attrs := fieldIssueAttrs("ecs")
	instances := []domain.Instance{
		{ID: 1, AssetID: "zz-r238-a", ModelUID: "ecs", Attributes: map[string]interface{}{"cpu": "abc"}},
		{ID: 2, AssetID: "zz-r238-b", ModelUID: "ecs", Attributes: map[string]interface{}{"status": "bogus"}},
		{ID: 3, AssetID: "zz-r238-c", ModelUID: "ecs", Attributes: map[string]interface{}{"cpu": "4", "status": "Running"}},
	}
	svc := newFieldIssueService(instances, attrs)
	ctx := context.Background()

	got, err := svc.FieldIssues(ctx, 3, "", "", 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 2 {
		t.Fatalf("期望 2 条字段值问题，实际 %d：%+v", got.Total, got.Items)
	}

	typed, err := svc.FieldIssues(ctx, 3, "", QualityIssueTypeFieldEnum, 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if typed.Total != 1 || typed.Items[0].FieldUID != "status" {
		t.Fatalf("按枚举筛选应只命中 status，实际 %+v", typed)
	}

	// 其它模型过滤后应为空（模型存在但无问题）
	empty, err := svc.FieldIssues(ctx, 3, "rds", "", 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("rds 无实例应返回空，实际 %+v", empty)
	}
}
