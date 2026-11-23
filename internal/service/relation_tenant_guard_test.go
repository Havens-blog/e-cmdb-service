package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// 回归背景（评审 R6，2026-11-23）：
// middleware.GetTenantID 对「未选择租户」的会话返回 0，而 instanceDAO.ListByIDs
// 只在 tenantID > 0 时才追加租户谓词。于是 tenant_id=0 的创建请求会跨所有租户
// 加载实例做模型对校验，并把关系持久化成 tenant_id=0 —— 既越权读，也写入一条
// 谁都不归属的关系。这里把「必须已选租户」下沉到 service 层。
func TestRelationCreate_RejectsUnselectedTenant(t *testing.T) {
	instRepo, relRepo := newPairFixture("ecs", "disk")
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	_, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 0, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk",
	})

	if !errors.Is(err, errs.ErrInvalidTenantID) {
		t.Fatalf("err = %v, want ErrInvalidTenantID", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("未选租户时不应写入关系，实际写入 %d 条", len(relRepo.created))
	}
}

func TestRelationCreateBatch_RejectsUnselectedTenant(t *testing.T) {
	instRepo, relRepo := newPairFixture("ecs", "disk")
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	_, err := svc.CreateBatch(context.Background(), []domain.InstanceRelation{
		{TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk"},
		{TenantID: 0, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk"},
	})

	if !errors.Is(err, errs.ErrInvalidTenantID) {
		t.Fatalf("err = %v, want ErrInvalidTenantID", err)
	}
}

// 历史数据里可能存在「声明为空」的关系类型（Validate 只拦新写入）。若照常比对，
// 任何真实实例都会被判 mismatch，等于把元数据瑕疵变成全量写入阻塞。
func TestRelationCreate_SkipsPairCheckWhenDeclarationEmpty(t *testing.T) {
	instRepo, relRepo := newPairFixture("host", "host")
	legacy := domain.ModelRelationType{ID: 3, UID: "legacy_type"}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(legacy))

	if _, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "legacy_type",
	}); err != nil {
		t.Fatalf("声明为空的关系类型应跳过模型对校验，err = %v", err)
	}
	if len(relRepo.created) != 1 {
		t.Fatalf("应写入 1 条关系，实际 %d 条", len(relRepo.created))
	}
}

// 声明为空只跳过「模型对」，端点存在性仍必须校验。
func TestRelationCreate_EmptyDeclarationStillRequiresEndpoints(t *testing.T) {
	instRepo, relRepo := newPairFixture("host", "host")
	legacy := domain.ModelRelationType{ID: 3, UID: "legacy_type"}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(legacy))

	_, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 999, RelationTypeUID: "legacy_type",
	})
	if !errors.Is(err, errs.ErrInstanceNotFound) {
		t.Fatalf("err = %v, want ErrInstanceNotFound", err)
	}
}

// 一次按租户分组后，跨租户批次仍要各自按本租户实例校验：
// 租户 3 的实例 id 不能出现在租户 4 的关系里。
func TestRelationCreateBatch_ValidatesPerTenant(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: "ecs"}
	instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 3, ModelUID: "disk"}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	// 同样两个实例 id，挂在租户 4 上就应视为不存在（不能跨租户读到）。
	_, err := svc.CreateBatch(context.Background(), []domain.InstanceRelation{
		{TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk"},
		{TenantID: 4, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk"},
	})
	if !errors.Is(err, errs.ErrInstanceNotFound) {
		t.Fatalf("跨租户批次应按本租户校验端点，err = %v", err)
	}
}
