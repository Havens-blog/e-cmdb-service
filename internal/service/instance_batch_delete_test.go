package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// TestBatchDelete 锁定批量删除的「逐条结果、不回滚」语义。
//
// 背景（2026-11-04）：产品批准先做批量删除，语义沿用关系第 6 条
// 「逐条结果，不回滚」——单条失败不影响其他条，成功与失败分别返回。
func TestBatchDelete(t *testing.T) {
	t.Run("全部存在则全部成功", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, AssetID: "a1"}
		instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 3, AssetID: "a2"}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil, nil, nil)
		res, err := svc.BatchDelete(context.Background(), 3, []int64{1, 2})
		if err != nil {
			t.Fatalf("BatchDelete 不应报错: %v", err)
		}
		if res.Total != 2 || len(res.Deleted) != 2 || len(res.Failed) != 0 {
			t.Fatalf("期望全部成功: %+v", res)
		}
	})

	t.Run("部分不存在则逐条失败、成功不回滚", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, AssetID: "a1"}
		// id 999 不存在

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil, nil, nil)
		res, err := svc.BatchDelete(context.Background(), 3, []int64{1, 999})
		if err != nil {
			t.Fatalf("BatchDelete 不应整体报错: %v", err)
		}
		if res.Total != 2 {
			t.Fatalf("Total 应为 2: %+v", res)
		}
		if len(res.Deleted) != 1 || res.Deleted[0] != 1 {
			t.Fatalf("id=1 应成功删除: %+v", res)
		}
		if len(res.Failed) != 1 || res.Failed[0].ID != 999 {
			t.Fatalf("id=999 应记失败: %+v", res)
		}
		if res.Failed[0].Error == "" {
			t.Fatal("失败应带错误信息")
		}
	})

	t.Run("跨租户 id 视为不存在（防误删）", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, AssetID: "a1"}
		instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 999, AssetID: "other-tenant"}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil, nil, nil)
		// 以 tenant 3 身份删 [1, 2]，id=2 属于 tenant 999，应失败
		res, err := svc.BatchDelete(context.Background(), 3, []int64{1, 2})
		if err != nil {
			t.Fatalf("不应整体报错: %v", err)
		}
		if len(res.Deleted) != 1 || res.Deleted[0] != 1 {
			t.Fatalf("仅 id=1 应成功: %+v", res)
		}
		if len(res.Failed) != 1 || res.Failed[0].ID != 2 {
			t.Fatalf("跨租户 id=2 应失败: %+v", res)
		}
	})

	t.Run("空 ids 返回空结果", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil, nil, nil)
		res, err := svc.BatchDelete(context.Background(), 3, nil)
		if err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if res.Total != 0 || len(res.Deleted) != 0 || len(res.Failed) != 0 {
			t.Fatalf("空输入应空结果: %+v", res)
		}
	})
}
