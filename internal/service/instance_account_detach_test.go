package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// TestInstanceDeleteByAccountID_CascadeRelations 锁定「按云账号删除实例时，按实例自身租户解除关系」。
//
// 背景（实测 2026-11-24，生产库）：质量页 dangling_relation_count 从 0 变 1，
// 具体是 relation_id=400 指向的 target 实例 385609 已被删除（GET 404），
// 而 source 实例 1965 仍存在 —— 删除实例的路径没有清掉这条关系。
//
// 代码根因：DeleteByAccountID 收集完实例后调用 detachRelations(ctx, 0, id)，
// 租户被硬编码成 0；而 dao.DeleteByInstanceID 的过滤条件是 `tenant_id: tenantID`
// 精确匹配，tenant 0 一条都删不掉。因此每次「按账号删除」（云账号移除、
// 资产重同步）都会把该批实例的全部关系留在库里，且删除主流程成功、
// 只在日志里留一条「已级联解除实例关系」，从现象上完全看不出来。
//
// 修复方式：连同实例自身的 tenant_id 一起收集，按各自租户解绑。
// 未采用「让 DAO 把 tenant 0 当通配」的方案 —— 那会扩大删除半径，
// 且让「忘记传租户」这种缺陷重新变成静默成功。
func TestInstanceDeleteByAccountID_CascadeRelations(t *testing.T) {
	t.Run("两个租户的实例各自按自身租户解除关系", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.listByAccount = map[int64][]domain.Instance{
			9: {
				{ID: 11, TenantID: 3, ModelUID: "ecs", AssetID: "i-11", AccountID: 9},
				{ID: 12, TenantID: 4, ModelUID: "ecs", AssetID: "i-12", AccountID: 9},
			},
		}
		relRepo := &fakeRelationRepoForCascade{}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, relRepo, nil, nil)
		if err := svc.DeleteByAccountID(context.Background(), 9); err != nil {
			t.Fatalf("按账号删除应成功，实际: %v", err)
		}

		if len(relRepo.deletedInstanceIDs) != 2 {
			t.Fatalf("两个实例都应解除关系，实际: %v", relRepo.deletedInstanceIDs)
		}
		wantIDs := map[int64]bool{11: true, 12: true}
		wantTenants := map[int64]int64{11: 3, 12: 4}
		for i, id := range relRepo.deletedInstanceIDs {
			tenant := relRepo.deletedTenants[i]
			if !wantIDs[id] {
				t.Fatalf("出现非预期实例 ID: %d", id)
			}
			if tenant != wantTenants[id] {
				t.Fatalf("实例 %d 应按租户 %d 解绑，实际: %d", id, wantTenants[id], tenant)
			}
			// 租户 0 是「忘记传租户」的特征值：DAO 按 tenant_id 精确匹配，
			// 传 0 等于什么都没删，且不会报错。
			if tenant == 0 {
				t.Fatalf("实例 %d 的解除关系租户为 0，DAO 将匹配不到任何关系", id)
			}
		}
	})

	t.Run("解除关系失败不阻断删除主流程", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.listByAccount = map[int64][]domain.Instance{
			9: {{ID: 11, TenantID: 3, ModelUID: "ecs", AssetID: "i-11", AccountID: 9}},
		}
		relRepo := &fakeRelationRepoForCascade{deleteErr: errors.New("mongo: connection refused")}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, relRepo, nil, nil)
		if err := svc.DeleteByAccountID(context.Background(), 9); err != nil {
			t.Fatalf("关系清理失败不应阻断删除（否则脏数据永远删不掉），实际: %v", err)
		}
		if len(relRepo.deletedInstanceIDs) != 1 || relRepo.deletedTenants[0] != 3 {
			t.Fatalf("仍应以实例自身租户尝试解绑，实际 ids=%v tenants=%v",
				relRepo.deletedInstanceIDs, relRepo.deletedTenants)
		}
	})

	t.Run("未注入关系仓储时跳过清理且不 panic", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.listByAccount = map[int64][]domain.Instance{
			9: {{ID: 11, TenantID: 3, ModelUID: "ecs", AssetID: "i-11", AccountID: 9}},
		}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil, nil, nil)
		if err := svc.DeleteByAccountID(context.Background(), 9); err != nil {
			t.Fatalf("未注入关系仓储时应直接删除，实际: %v", err)
		}
	})
}
