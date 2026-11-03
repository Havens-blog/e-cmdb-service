package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// TestInstanceValidationService 锁定「Create 硬拦必填 / Update 不拦」的 service 层行为。
//
// 背景（2026-11-03）：PR #33 把 validateModelAttributes 接入 Create（blockRequired=true，
// 硬拦）与 Update（blockRequired=false，仅警告），但只有 domain 层 ValidateAttributes
// 纯函数测试，无 service 层测试锁定「Create 拒绝、Update 放行」这一核心差异。
func TestInstanceValidationService(t *testing.T) {
	ecsDefs := []domain.Attribute{
		{ModelUID: "ecs", FieldUID: "instance_id", FieldType: "string", Required: true},
		{ModelUID: "ecs", FieldUID: "status", FieldType: "enum", Required: true, Option: `["Running"]`},
	}

	t.Run("Create 缺必填字段硬拦", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		attrRepo := &fakeAttributeRepoForSync{defs: ecsDefs}

		svc := NewInstanceService(instRepo, &fakeModelRepo{getByUID: domain.Model{ID: 1}}, attrRepo, nil, nil, nil)

		_, err := svc.Create(context.Background(), domain.Instance{
			TenantID: 3, ModelUID: "ecs", AssetID: "i-bad",
			Attributes: map[string]interface{}{"status": "Running"}, // 缺 instance_id
		})
		if err == nil {
			t.Fatal("缺必填字段应被拒绝")
		}
		if !errors.Is(err, errs.ErrInvalidAttributes) {
			t.Fatalf("应为 ErrInvalidAttributes，实际: %v", err)
		}
	})

	t.Run("Create 补齐必填成功", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		attrRepo := &fakeAttributeRepoForSync{defs: ecsDefs}

		svc := NewInstanceService(instRepo, &fakeModelRepo{getByUID: domain.Model{ID: 1}}, attrRepo, nil, nil, nil)

		_, err := svc.Create(context.Background(), domain.Instance{
			TenantID: 3, ModelUID: "ecs", AssetID: "i-good",
			Attributes: map[string]interface{}{"instance_id": "i-good", "status": "Running"},
		})
		if err != nil {
			t.Fatalf("补齐必填应成功: %v", err)
		}
	})

	t.Run("Update 缺必填不拦（存量可编辑）", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		instRepo.instances[7] = domain.Instance{ID: 7, TenantID: 3, ModelUID: "ecs", AssetID: "i-7"}
		attrRepo := &fakeAttributeRepoForSync{defs: ecsDefs}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, attrRepo, nil, nil, nil)

		// 更新只传一个字段，缺全部必填 → 不应拦截
		err := svc.Update(context.Background(), 3, domain.Instance{
			ID: 7, TenantID: 3, ModelUID: "ecs", AssetID: "i-7",
			Attributes: map[string]interface{}{"cpu": 4},
		})
		if err != nil {
			t.Fatalf("更新缺必填不应拦截（存量可编辑）: %v", err)
		}
	})
}
