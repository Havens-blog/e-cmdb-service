package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// fakeRelationRepoForDerive 记录 Create 调用，可配置 Exists 返回。
type fakeRelationRepoForDerive struct {
	created     []domain.InstanceRelation
	existsValue bool
}

func (f *fakeRelationRepoForDerive) Create(ctx context.Context, r domain.InstanceRelation) (int64, error) {
	f.created = append(f.created, r)
	return 0, nil
}
func (f *fakeRelationRepoForDerive) CreateBatch(ctx context.Context, rs []domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepoForDerive) GetByID(ctx context.Context, tenantID, id int64) (domain.InstanceRelation, error) {
	return domain.InstanceRelation{}, nil
}
func (f *fakeRelationRepoForDerive) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error) {
	return nil, nil
}
func (f *fakeRelationRepoForDerive) Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepoForDerive) Delete(ctx context.Context, tenantID, id int64) error { return nil }
func (f *fakeRelationRepoForDerive) DeleteByInstanceID(ctx context.Context, tenantID, instanceID int64) error {
	return nil
}
func (f *fakeRelationRepoForDerive) Exists(ctx context.Context, tenantID, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	return f.existsValue, nil
}

var _ repository.InstanceRelationRepository = (*fakeRelationRepoForDerive)(nil)

// TestDeriveRelationsService 锁定「推导实例关系」的 service 层行为。
//
// 背景（2026-11-03）：PR #34 实现 DeriveRelations（dry-run 只统计、write 实际写入、幂等），
// 但只有 domain 层 DeriveEcsDisk 纯函数测试，无 service 层测试锁定 dry-run/write/幂等。
func TestDeriveRelationsService(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.listByModel = map[string][]domain.Instance{
		"ecs":  {{ID: 100, AssetID: "i-a", ModelUID: "ecs"}},
		"disk": {{ID: 200, AssetID: "d-1", ModelUID: "disk", Attributes: map[string]interface{}{"instance_id": "i-a"}}},
	}

	t.Run("dry-run 不写入", func(t *testing.T) {
		relRepo := &fakeRelationRepoForDerive{}
		svc := NewRelationService(relRepo, instRepo)

		res, err := svc.DeriveRelations(context.Background(), 3, "ecs_disk", true)
		if err != nil {
			t.Fatalf("dry-run 应成功: %v", err)
		}
		if len(relRepo.created) != 0 {
			t.Fatalf("dry-run 不应写入，实际 %d 条", len(relRepo.created))
		}
		if len(res.Rules) != 1 || res.Rules[0].Candidate != 1 {
			t.Fatalf("dry-run 应统计 1 个候选: %+v", res.Rules)
		}
	})

	t.Run("write 实际写入", func(t *testing.T) {
		relRepo := &fakeRelationRepoForDerive{}
		svc := NewRelationService(relRepo, instRepo)

		res, err := svc.DeriveRelations(context.Background(), 3, "ecs_disk", false)
		if err != nil {
			t.Fatalf("write 应成功: %v", err)
		}
		if len(relRepo.created) != 1 {
			t.Fatalf("应写入 1 条，实际 %d", len(relRepo.created))
		}
		if relRepo.created[0].SourceInstanceID != 100 || relRepo.created[0].TargetInstanceID != 200 {
			t.Fatalf("方向错误: %+v", relRepo.created[0])
		}
		if res.Total != 1 {
			t.Fatalf("Total 应为 1: %+v", res)
		}
	})

	t.Run("已存在则幂等跳过", func(t *testing.T) {
		relRepo := &fakeRelationRepoForDerive{existsValue: true}
		svc := NewRelationService(relRepo, instRepo)

		res, err := svc.DeriveRelations(context.Background(), 3, "ecs_disk", false)
		if err != nil {
			t.Fatalf("应成功: %v", err)
		}
		if len(relRepo.created) != 0 {
			t.Fatalf("已存在不应再写入，实际 %d", len(relRepo.created))
		}
		if res.Rules[0].SkippedExisting != 1 {
			t.Fatalf("SkippedExisting 应为 1: %+v", res.Rules[0])
		}
	})

	t.Run("不支持的规则返回错误", func(t *testing.T) {
		relRepo := &fakeRelationRepoForDerive{}
		svc := NewRelationService(relRepo, instRepo)

		_, err := svc.DeriveRelations(context.Background(), 3, "vpc_subnet", false)
		if err == nil {
			t.Fatal("不支持的规则应报错")
		}
	})
}
