package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
)

// upsertAuditRepo 让 fakeInstanceRepo 支持「按 (tenant, model, asset) 返回既有实例」，
// 用于验证 upsert 的变更审计能拿到旧属性。
type upsertAuditRepo struct {
	*fakeInstanceRepo
	existing map[string]domain.Instance
	upserted []domain.Instance
}

func (r *upsertAuditRepo) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error) {
	key := modelUID + "|" + assetID
	if inst, ok := r.existing[key]; ok {
		return inst, nil
	}
	// 与真实 DAO 一致：未命中返回错误
	return domain.Instance{}, errors.New("mongo: no documents in result")
}

func (r *upsertAuditRepo) Upsert(ctx context.Context, instance domain.Instance) error {
	r.upserted = append(r.upserted, instance)
	return nil
}

// TestInstanceAuditUpsert 锁定「导入（upsert / upsert-batch）也要写变更审计」。
//
// 背景（2026-11-19）：Create / Update / Delete 都写 ecam_change_history，唯独
// Upsert / UpsertBatch 不写；而 e-cmdb 的 upsert 唯一调用方是「导入实例」，
// 于是导入进来的资产在变更审计里完全不可见（实测 changes total=0）。
func TestInstanceAuditUpsert(t *testing.T) {
	newRepoSvc := func(existing map[string]domain.Instance) (*upsertAuditRepo, *fakeChangeHistoryRepo, InstanceService) {
		repo := &upsertAuditRepo{fakeInstanceRepo: newFakeInstanceRepo(), existing: existing}
		chRepo := &fakeChangeHistoryRepo{}
		svc := NewInstanceService(repo, &fakeModelRepo{getByUID: domain.Model{ID: 1}}, nil, nil, nil, chRepo)
		return repo, chRepo, svc
	}

	t.Run("upsert 新增：全部字段记为 import 来源的新增", func(t *testing.T) {
		_, chRepo, svc := newRepoSvc(nil)
		ctx := middleware.WithOperator(context.Background(), "zhangsan")

		err := svc.Upsert(ctx, domain.Instance{
			TenantID:   3,
			ModelUID:   "ecs",
			AssetID:    "i-import-new",
			AssetName:  "导入实例",
			Source:     "import",
			Attributes: map[string]interface{}{"cpu": 4, "status": "Running"},
		})
		if err != nil {
			t.Fatalf("upsert 应成功: %v", err)
		}
		if len(chRepo.records) == 0 {
			t.Fatal("upsert 新增应写审计，实际 0 条")
		}
		for _, r := range chRepo.records {
			if r.ChangeSource != domain.ChangeSourceImport {
				t.Fatalf("导入来源应为 import: %+v", r)
			}
			if r.OldValue != "" {
				t.Fatalf("新增字段 OldValue 应为空: %+v", r)
			}
			if r.AssetID != "i-import-new" || r.TenantID != 3 || r.ModelUID != "ecs" {
				t.Fatalf("审计元数据错误: %+v", r)
			}
			if r.Operator != "zhangsan" {
				t.Fatalf("操作者应记录为 zhangsan: %+v", r)
			}
		}
	})

	t.Run("upsert 更新：只记真正变化的字段并带旧值", func(t *testing.T) {
		existing := map[string]domain.Instance{
			"ecs|i-import-upd": {
				ID:         9,
				TenantID:   3,
				ModelUID:   "ecs",
				AssetID:    "i-import-upd",
				Attributes: map[string]interface{}{"cpu": 4, "status": "Running", "note": "same"},
			},
		}
		_, chRepo, svc := newRepoSvc(existing)

		err := svc.Upsert(context.Background(), domain.Instance{
			TenantID:   3,
			ModelUID:   "ecs",
			AssetID:    "i-import-upd",
			Attributes: map[string]interface{}{"cpu": 8, "status": "Running", "note": "same"},
		})
		if err != nil {
			t.Fatalf("upsert 应成功: %v", err)
		}
		if len(chRepo.records) != 1 {
			t.Fatalf("应只记录 cpu 一个字段变化，实际 %d 条: %+v", len(chRepo.records), chRepo.records)
		}
		rec := chRepo.records[0]
		if rec.FieldName != "cpu" {
			t.Fatalf("变更字段应为 cpu: %+v", rec)
		}
		if rec.ChangeSource != domain.ChangeSourceImport {
			t.Fatalf("导入来源应为 import: %+v", rec)
		}
		if rec.OldValue == "" || rec.NewValue == "" {
			t.Fatalf("更新必须同时记录旧值与新值: %+v", rec)
		}
	})

	t.Run("upsert 无变化不写审计", func(t *testing.T) {
		existing := map[string]domain.Instance{
			"ecs|i-same": {
				ID:         10,
				TenantID:   3,
				ModelUID:   "ecs",
				AssetID:    "i-same",
				Attributes: map[string]interface{}{"cpu": 4},
			},
		}
		_, chRepo, svc := newRepoSvc(existing)

		err := svc.Upsert(context.Background(), domain.Instance{
			TenantID:   3,
			ModelUID:   "ecs",
			AssetID:    "i-same",
			Attributes: map[string]interface{}{"cpu": 4},
		})
		if err != nil {
			t.Fatalf("upsert 应成功: %v", err)
		}
		if len(chRepo.records) != 0 {
			t.Fatalf("属性无变化不应产生审计记录，实际 %d 条", len(chRepo.records))
		}
	})

	t.Run("upsert-batch 逐条写审计", func(t *testing.T) {
		_, chRepo, svc := newRepoSvc(nil)

		err := svc.UpsertBatch(context.Background(), []domain.Instance{
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-b1", Attributes: map[string]interface{}{"cpu": 1}},
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-b2", Attributes: map[string]interface{}{"cpu": 2}},
		})
		if err != nil {
			t.Fatalf("批量 upsert 应成功: %v", err)
		}
		assets := map[string]int{}
		for _, r := range chRepo.records {
			if r.ChangeSource != domain.ChangeSourceImport {
				t.Fatalf("导入来源应为 import: %+v", r)
			}
			assets[r.AssetID]++
		}
		if assets["i-b1"] != 1 || assets["i-b2"] != 1 {
			t.Fatalf("两行都应各写 1 条审计，实际 %+v", assets)
		}
	})

	t.Run("未注入审计仓储时不 panic", func(t *testing.T) {
		repo := &upsertAuditRepo{fakeInstanceRepo: newFakeInstanceRepo()}
		svc := NewInstanceService(repo, &fakeModelRepo{getByUID: domain.Model{ID: 1}}, nil, nil, nil, nil)
		if err := svc.Upsert(context.Background(), domain.Instance{
			TenantID: 3, ModelUID: "ecs", AssetID: "i-noaudit", Attributes: map[string]interface{}{"cpu": 1},
		}); err != nil {
			t.Fatalf("无审计仓储时 upsert 仍应成功: %v", err)
		}
		if len(repo.upserted) != 1 {
			t.Fatalf("应完成写入，实际 %d 条", len(repo.upserted))
		}
	})
}
