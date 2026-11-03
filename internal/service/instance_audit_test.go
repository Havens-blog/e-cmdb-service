package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// fakeChangeHistoryRepo 记录 BatchCreate 的调用，用于验证审计写入。
type fakeChangeHistoryRepo struct {
	records []domain.ChangeRecord
	err     error
}

func (f *fakeChangeHistoryRepo) BatchCreate(ctx context.Context, records []domain.ChangeRecord) error {
	f.records = append(f.records, records...)
	return f.err
}

var _ repository.ChangeHistoryRepository = (*fakeChangeHistoryRepo)(nil)

// TestInstanceAudit 锁定实例 Create/Update/Delete 的变更审计写入。
//
// 背景（2026-11-03）：PR #38 给实例 CRUD 补了变更审计（写入 ecam_change_history），
// 但只有 domain 层 DiffAttributes 的纯函数测试，**没有 service 层测试锁定**
// 「Create/Update/Delete 确实以正确的 old/new 调用审计写入」这一核心行为。
// 本测试补齐回归守卫。
func TestInstanceAudit(t *testing.T) {
	t.Run("Create 记录全部字段为新增（OldValue 空）", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		chRepo := &fakeChangeHistoryRepo{}

		svc := NewInstanceService(instRepo, &fakeModelRepo{getByUID: domain.Model{ID: 1}}, nil, nil, nil, chRepo)

		_, err := svc.Create(context.Background(), domain.Instance{
			TenantID:   3,
			ModelUID:   "huawei_ecs",
			AssetID:    "i-new",
			AssetName:  "新实例",
			Attributes: map[string]interface{}{"note": "v1", "cpu": 2},
		})
		if err != nil {
			t.Fatalf("创建应成功: %v", err)
		}

		if len(chRepo.records) == 0 {
			t.Fatal("Create 应写入审计，实际 0 条")
		}
		for _, r := range chRepo.records {
			if r.ChangeSource != domain.ChangeSourceManual {
				t.Fatalf("手工创建 change_source 应为 manual: %+v", r)
			}
			if r.OldValue != "" {
				t.Fatalf("新增字段 OldValue 应为空: %+v", r)
			}
			if r.AssetID != "i-new" || r.TenantID != 3 {
				t.Fatalf("审计元数据错误: %+v", r)
			}
		}
	})

	t.Run("Update 记录字段 diff（旧值 vs 新值）", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[7] = domain.Instance{
			ID: 7, TenantID: 3, ModelUID: "huawei_ecs", AssetID: "i-7", AssetName: "旧名",
			Attributes: map[string]interface{}{"note": "v1", "cpu": 2},
		}
		chRepo := &fakeChangeHistoryRepo{}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil, nil, chRepo)

		err := svc.Update(context.Background(), 3, domain.Instance{
			ID: 7, TenantID: 3, ModelUID: "huawei_ecs", AssetID: "i-7", AssetName: "旧名",
			Attributes: map[string]interface{}{"note": "v2", "cpu": 8},
		})
		if err != nil {
			t.Fatalf("更新应成功: %v", err)
		}

		// 应记录 note 和 cpu 两条变更（旧值 → 新值）
		if len(chRepo.records) != 2 {
			t.Fatalf("期望 2 条变更记录，实际 %d: %+v", len(chRepo.records), chRepo.records)
		}
		fields := map[string]domain.ChangeRecord{}
		for _, r := range chRepo.records {
			fields[r.FieldName] = r
		}
		if fields["note"].OldValue == "" || fields["note"].NewValue == "" {
			t.Fatalf("note 变更应有旧值和新值: %+v", fields["note"])
		}
		if fields["cpu"].OldValue == "" || fields["cpu"].NewValue == "" {
			t.Fatalf("cpu 变更应有旧值和新值: %+v", fields["cpu"])
		}
	})

	t.Run("Delete 记录全部字段为删除（NewValue 空）", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[9] = domain.Instance{
			ID: 9, TenantID: 3, ModelUID: "huawei_ecs", AssetID: "i-9", AssetName: "待删",
			Attributes: map[string]interface{}{"note": "v1"},
		}
		chRepo := &fakeChangeHistoryRepo{}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, nil, nil, nil, chRepo)

		if err := svc.Delete(context.Background(), 3, 9); err != nil {
			t.Fatalf("删除应成功: %v", err)
		}

		if len(chRepo.records) == 0 {
			t.Fatal("Delete 应写入审计，实际 0 条")
		}
		for _, r := range chRepo.records {
			if r.NewValue != "" {
				t.Fatalf("删除字段 NewValue 应为空: %+v", r)
			}
			if r.ChangeSource != domain.ChangeSourceManual {
				t.Fatalf("手工删除 change_source 应为 manual: %+v", r)
			}
		}
	})

	t.Run("审计写入失败不阻断操作（非阻断）", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[11] = domain.Instance{ID: 11, TenantID: 3, ModelUID: "huawei_ecs", AssetID: "i-11"}
		chRepo := &fakeChangeHistoryRepo{err: context.DeadlineExceeded}

		svc := NewInstanceService(instRepo, &fakeModelRepo{getByUID: domain.Model{ID: 1}}, nil, nil, nil, chRepo)

		// Create 失败不阻断
		_, err := svc.Create(context.Background(), domain.Instance{TenantID: 3, ModelUID: "huawei_ecs", AssetID: "i-new2", Attributes: map[string]interface{}{"a": 1}})
		if err != nil {
			t.Fatalf("审计失败不应阻断创建: %v", err)
		}

		// Delete 失败不阻断
		if err := svc.Delete(context.Background(), 3, 11); err != nil {
			t.Fatalf("审计失败不应阻断删除: %v", err)
		}
	})
}
