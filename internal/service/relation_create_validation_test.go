package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// 回归背景（实测 2026-11-17）：Create 仅查重、不校验端点实例是否存在，
// 历史产生 8 条两端皆亡的悬空关系（数据质量 dangling_relation）。
// 下列用例锁定「写入前校验两端实例（同租户）」的口径。

func TestRelationCreate_RejectsMissingEndpoint(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: "ecs"}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	_, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk",
	})

	if !errors.Is(err, errs.ErrInstanceNotFound) {
		t.Fatalf("err = %v, want ErrInstanceNotFound", err)
	}
	if !strings.Contains(err.Error(), "instance_id=2") {
		t.Fatalf("错误信息应指明缺失实例: %v", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("端点缺失时不应写入关系，实际写入 %d 条", len(relRepo.created))
	}
}

func TestRelationCreate_AllowsExistingEndpoints(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: "ecs"}
	instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 3, ModelUID: "disk"}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	if _, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk",
	}); err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}
	if len(relRepo.created) != 1 {
		t.Fatalf("应写入 1 条关系，实际 %d", len(relRepo.created))
	}
}

func TestRelationCreate_RejectsCrossTenantEndpoint(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: "ecs"}
	instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 99, ModelUID: "disk"} // 另一租户
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	_, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk",
	})
	if !errors.Is(err, errs.ErrInstanceNotFound) {
		t.Fatalf("跨租户端点应按不存在拒绝，err = %v", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("跨租户时不应写入关系")
	}
}

func TestRelationCreateBatch_RejectsWhenAnyMissing(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: "ecs"}
	instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 3, ModelUID: "disk"}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	_, err := svc.CreateBatch(context.Background(), []domain.InstanceRelation{
		{TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk"},
		{TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 404, RelationTypeUID: "ecs_disk"},
	})
	if !errors.Is(err, errs.ErrInstanceNotFound) {
		t.Fatalf("批量含缺失端点应整体拒绝，err = %v", err)
	}
}

func TestRelationCreateBatch_AllowsExistingEndpoints(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: "ecs"}
	instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 3, ModelUID: "disk"}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	if _, err := svc.CreateBatch(context.Background(), []domain.InstanceRelation{
		{TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk"},
	}); err != nil {
		t.Fatalf("CreateBatch err = %v, want nil", err)
	}
}
