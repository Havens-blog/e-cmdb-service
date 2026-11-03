package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// fakeRelationRepoForList 返回固定关系列表的 relation repo。
type fakeRelationRepoForList struct {
	relations []domain.InstanceRelation
	total     int64
	listErr   error
	countErr  error
}

func (f *fakeRelationRepoForList) Create(ctx context.Context, r domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepoForList) CreateBatch(ctx context.Context, rs []domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepoForList) GetByID(ctx context.Context, tenantID, id int64) (domain.InstanceRelation, error) {
	return domain.InstanceRelation{}, nil
}
func (f *fakeRelationRepoForList) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error) {
	return f.relations, f.listErr
}
func (f *fakeRelationRepoForList) Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	return f.total, f.countErr
}
func (f *fakeRelationRepoForList) Delete(ctx context.Context, tenantID, id int64) error {
	return nil
}
func (f *fakeRelationRepoForList) DeleteByInstanceID(ctx context.Context, tenantID, instanceID int64) error {
	return nil
}
func (f *fakeRelationRepoForList) Exists(ctx context.Context, tenantID, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	return false, nil
}

var _ repository.InstanceRelationRepository = (*fakeRelationRepoForList)(nil)

// TestListDetailed 锁定「实例关系列表带两端资产摘要」的批量解析行为。
//
// 背景（2026-11-03）：PR #36 实现 ListDetailed（批量解析两端实例 → asset 摘要），
// 但无 service 层测试。本测试锁定：正常解析、端点缺失不阻断、解析失败降级、去重。
func TestListDetailed(t *testing.T) {
	t.Run("两端实例都解析出摘要", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[100] = domain.Instance{ID: 100, TenantID: 3, ModelUID: "ecs", AssetID: "i-a", AssetName: "源实例"}
		instRepo.instances[200] = domain.Instance{ID: 200, TenantID: 3, ModelUID: "disk", AssetID: "d-1", AssetName: "目标盘"}

		relRepo := &fakeRelationRepoForList{
			relations: []domain.InstanceRelation{
				{ID: 1, SourceInstanceID: 100, TargetInstanceID: 200, RelationTypeUID: "ecs_disk", TenantID: 3},
			},
			total: 1,
		}

		svc := NewRelationService(relRepo, instRepo)
		details, total, err := svc.ListDetailed(context.Background(), domain.InstanceRelationFilter{TenantID: 3})
		if err != nil {
			t.Fatalf("ListDetailed 应成功: %v", err)
		}
		if total != 1 || len(details) != 1 {
			t.Fatalf("total=%d len=%d, 期望 1/1", total, len(details))
		}
		d := details[0]
		if d.Source.AssetID != "i-a" || d.Source.AssetName != "源实例" || d.Source.ModelUID != "ecs" {
			t.Fatalf("源摘要错误: %+v", d.Source)
		}
		if d.Target.AssetID != "d-1" || d.Target.ModelUID != "disk" {
			t.Fatalf("目标摘要错误: %+v", d.Target)
		}
	})

	t.Run("端点实例不存在时摘要为空但不报错", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[100] = domain.Instance{ID: 100, TenantID: 3, ModelUID: "ecs", AssetID: "i-a"}

		relRepo := &fakeRelationRepoForList{
			relations: []domain.InstanceRelation{
				// target 999 不存在
				{ID: 2, SourceInstanceID: 100, TargetInstanceID: 999, RelationTypeUID: "ecs_disk", TenantID: 3},
			},
			total: 1,
		}

		svc := NewRelationService(relRepo, instRepo)
		details, _, err := svc.ListDetailed(context.Background(), domain.InstanceRelationFilter{TenantID: 3})
		if err != nil {
			t.Fatalf("端点缺失不应报错: %v", err)
		}
		if details[0].Source.AssetID != "i-a" {
			t.Fatalf("源应正常解析: %+v", details[0].Source)
		}
		if details[0].Target.ID != 0 {
			t.Fatalf("不存在的目标应返回零值: %+v", details[0].Target)
		}
	})

	t.Run("批量解析失败降级为无摘要，但不阻断", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		relRepo := &fakeRelationRepoForList{
			relations: []domain.InstanceRelation{
				{ID: 3, SourceInstanceID: 100, TargetInstanceID: 200, RelationTypeUID: "ecs_disk", TenantID: 3},
			},
			total: 1,
		}

		svc := NewRelationService(relRepo, instRepo)
		details, total, err := svc.ListDetailed(context.Background(), domain.InstanceRelationFilter{TenantID: 3})
		if err != nil {
			t.Fatalf("解析失败不应报错: %v", err)
		}
		if total != 1 || len(details) != 1 {
			t.Fatalf("应正常返回列表，total=%d len=%d", total, len(details))
		}
		// instRepo 里没有 100/200，ListByIDs 返回空 → 摘要应为零值
		if details[0].Source.ID != 0 {
			t.Fatalf("无实例时应零值摘要: %+v", details[0].Source)
		}
	})

	t.Run("同端点去重：多个关系共享端点时正确复用摘要", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[100] = domain.Instance{ID: 100, TenantID: 3, ModelUID: "ecs", AssetID: "i-a", AssetName: "源"}
		instRepo.instances[200] = domain.Instance{ID: 200, TenantID: 3, ModelUID: "disk", AssetID: "d-1"}
		instRepo.instances[201] = domain.Instance{ID: 201, TenantID: 3, ModelUID: "disk", AssetID: "d-2"}

		relRepo := &fakeRelationRepoForList{
			relations: []domain.InstanceRelation{
				{ID: 4, SourceInstanceID: 100, TargetInstanceID: 200, RelationTypeUID: "ecs_disk", TenantID: 3},
				{ID: 5, SourceInstanceID: 100, TargetInstanceID: 201, RelationTypeUID: "ecs_disk", TenantID: 3},
			},
			total: 2,
		}

		svc := NewRelationService(relRepo, instRepo)
		details, _, err := svc.ListDetailed(context.Background(), domain.InstanceRelationFilter{TenantID: 3})
		if err != nil {
			t.Fatalf("ListDetailed 应成功: %v", err)
		}
		if len(details) != 2 {
			t.Fatalf("期望 2 条，实际 %d", len(details))
		}
		// 两条共享 source 100，都应有源摘要
		for _, d := range details {
			if d.Source.AssetID != "i-a" {
				t.Fatalf("共享端点摘要应一致: %+v", d.Source)
			}
		}
		// target 各不同
		if details[0].Target.AssetID == details[1].Target.AssetID {
			t.Fatal("不同 target 摘要不应相同")
		}
	})
}
