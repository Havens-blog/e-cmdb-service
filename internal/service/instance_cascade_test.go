package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// fakeRelationRepoForCascade 记录 DeleteByInstanceID 的调用情况
type fakeRelationRepoForCascade struct {
	deletedInstanceIDs []int64
	deletedTenants     []int64
	deleteErr          error
}

func (f *fakeRelationRepoForCascade) Create(ctx context.Context, r domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepoForCascade) CreateBatch(ctx context.Context, rs []domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepoForCascade) GetByID(ctx context.Context, tenantID, id int64) (domain.InstanceRelation, error) {
	return domain.InstanceRelation{}, nil
}
func (f *fakeRelationRepoForCascade) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error) {
	return nil, nil
}
func (f *fakeRelationRepoForCascade) Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepoForCascade) Delete(ctx context.Context, tenantID, id int64) error {
	return nil
}
func (f *fakeRelationRepoForCascade) DeleteByInstanceID(ctx context.Context, tenantID, instanceID int64) error {
	f.deletedInstanceIDs = append(f.deletedInstanceIDs, instanceID)
	f.deletedTenants = append(f.deletedTenants, tenantID)
	return f.deleteErr
}
func (f *fakeRelationRepoForCascade) Exists(ctx context.Context, tenantID, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	return false, nil
}

// TestInstanceDelete_CascadeRelations 锁定「删除实例时级联解除关系」。
//
// 背景（实测 2026-11-02）：此前 Delete 只删实例、完全不动关系。
// 实测复现：建 A→B 关系后删除 B，关系仍残留（target 指向不存在的实例），
// 即产生悬空关系，违反已确认规则第 5 条「删除资产时级联解除关系」。
func TestInstanceDelete_CascadeRelations(t *testing.T) {
	t.Run("删除实例后按正确的租户与实例 ID 解除关系", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[7] = domain.Instance{ID: 7, TenantID: 3, ModelUID: "ecs", AssetID: "i-7"}
		relRepo := &fakeRelationRepoForCascade{}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, relRepo)
		if err := svc.Delete(context.Background(), 3, 7); err != nil {
			t.Fatalf("删除应成功，实际: %v", err)
		}
		if len(relRepo.deletedInstanceIDs) != 1 || relRepo.deletedInstanceIDs[0] != 7 {
			t.Fatalf("应以实例 ID 7 解除关系，实际: %v", relRepo.deletedInstanceIDs)
		}
		if relRepo.deletedTenants[0] != 3 {
			t.Fatalf("租户应为 3，实际: %v", relRepo.deletedTenants[0])
		}
		if instRepo.deletedID != 7 || instRepo.deletedTenant != 3 {
			t.Fatalf("应以租户 3 删除实例 7，实际 tenant=%d id=%d", instRepo.deletedTenant, instRepo.deletedID)
		}
	})

	t.Run("实例不存在时不解除任何关系", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		relRepo := &fakeRelationRepoForCascade{}
		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, relRepo)

		if err := svc.Delete(context.Background(), 3, 999); err == nil {
			t.Fatal("不存在的实例应返回错误")
		}
		if len(relRepo.deletedInstanceIDs) != 0 {
			t.Fatalf("不应解除任何关系，实际: %v", relRepo.deletedInstanceIDs)
		}
	})

	t.Run("关系清理失败不阻断删除（否则脏数据永远删不掉）", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[8] = domain.Instance{ID: 8, TenantID: 3, ModelUID: "ecs", AssetID: "i-8"}
		relRepo := &fakeRelationRepoForCascade{deleteErr: context.DeadlineExceeded}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, relRepo)
		if err := svc.Delete(context.Background(), 3, 8); err != nil {
			t.Fatalf("关系清理失败不应导致删除失败，实际: %v", err)
		}
		if instRepo.deletedID != 8 {
			t.Fatalf("关系清理失败时实例仍应被删除，实际 deletedID=%d", instRepo.deletedID)
		}
	})

	t.Run("relationRepo 为 nil 时不 panic", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[9] = domain.Instance{ID: 9, TenantID: 3, ModelUID: "ecs", AssetID: "i-9"}
		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil)

		if err := svc.Delete(context.Background(), 3, 9); err != nil {
			t.Fatalf("未注入 relationRepo 时应能正常删除，实际: %v", err)
		}
	})
}
