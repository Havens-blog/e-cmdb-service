package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// TestDeriveRelations_RejectsUnselectedTenant 锁定推导路径的租户守卫。
//
// 缺陷背景（评审指出，2026-11-23）：租户守卫此前只加在 Create/CreateBatch，
// DeriveRelations 用调用方传进来的 tenantID 直接读实例并写关系，tenant_id=0 时
// instanceDAO.List 不追加租户谓词（跨租户读）且写入的关系无归属。
func TestDeriveRelations_RejectsUnselectedTenant(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	for _, tenantID := range []int64{0, -1} {
		if _, err := svc.DeriveRelations(context.Background(), tenantID, "ecs_disk", true); !errors.Is(err, errs.ErrInvalidTenantID) {
			t.Fatalf("tenant_id=%d 应被拒: got %v", tenantID, err)
		}
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("被拒时不应写入，实际写入 %d 条", len(relRepo.created))
	}
}

// TestDeriveRelations_SkipsCandidatesBreakingDeclaration 锁定「推导不再是校验旁路」。
//
// 缺陷背景（评审指出，2026-11-23）：DeriveRelations 直接 s.repo.Create，绕过
// Create/CreateBatch 的模型对校验。若 ecs_disk 的声明被改成别的模型对（或声明缺失后
// 又被补成非 ecs→disk），POST 会拒绝但推导仍能落库，本 PR 想关掉的入口等于还开着。
// 语义选择：推导是批量修复动作，单条不合法跳过并留痕（SkippedInvalid），不整批回滚。
func TestDeriveRelations_SkipsCandidatesBreakingDeclaration(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.listByModel = map[string][]domain.Instance{
		"ecs":  {{ID: 100, AssetID: "i-a", ModelUID: "ecs"}},
		"disk": {{ID: 200, AssetID: "d-1", ModelUID: "disk", Attributes: map[string]interface{}{"instance_id": "i-a"}}},
	}
	// 声明被改成 vpc→subnet：候选用的是 ecs→disk，必然不匹配。
	declared := domain.ModelRelationType{ID: 9, UID: "ecs_disk", SourceModelUID: "vpc", TargetModelUID: "subnet"}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(declared))

	res, err := svc.DeriveRelations(context.Background(), 3, "ecs_disk", false)
	if err != nil {
		t.Fatalf("单条不合法不应让整批失败: %v", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("不符合声明的关系不应写入，实际写入 %d 条", len(relRepo.created))
	}
	if len(res.Rules) != 1 || res.Rules[0].SkippedInvalid != 1 {
		t.Fatalf("应把跳过条数计入统计: %+v", res.Rules)
	}
}

// TestDeriveRelations_StoresCandidatesMatchingDeclaration 是上一条的对照：
// 声明与候选一致时推导必须照常写入，证明新增校验没有把正常路径一起拦住。
func TestDeriveRelations_StoresCandidatesMatchingDeclaration(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.listByModel = map[string][]domain.Instance{
		"ecs":  {{ID: 100, AssetID: "i-a", ModelUID: "ecs"}},
		"disk": {{ID: 200, AssetID: "d-1", ModelUID: "disk", Attributes: map[string]interface{}{"instance_id": "i-a"}}},
	}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	res, err := svc.DeriveRelations(context.Background(), 3, "ecs_disk", false)
	if err != nil {
		t.Fatalf("正常推导应成功: %v", err)
	}
	if len(relRepo.created) != 1 || relRepo.created[0].TenantID != 3 {
		t.Fatalf("应写入 1 条且带租户: %+v", relRepo.created)
	}
	if res.Rules[0].SkippedInvalid != 0 {
		t.Fatalf("声明匹配时不应有跳过: %+v", res.Rules)
	}
}

// TestDeriveRelations_NoModelRelRepoStillWrites 锁定「依赖漏注入不得拦住正常推导」。
//
// 缺陷背景（评审指出，2026-11-24）：validateDeriveCandidate 的降级条件与注释相悖——
// 注释说「只缺一个仍能做一半校验」，实现却是 instanceRepo 或 modelRelRepo 任一为 nil
// 就整体跳过。两个依赖缺失时都只能跳过（instanceRepo 缺失时 models 为空表、硬比对会
// 误拒一切；modelRelRepo 缺失时拿不到声明），因此行为本身正确，需要锁定的是：
// 降级时**不得**把候选判成 skipped_invalid（那会让推导静默无产出），并且两条降级
// 路径各自留痕（日志无法在单测里断言，这里用行为锁定可见结果）。
func TestDeriveRelations_NoModelRelRepoStillWrites(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.listByModel = map[string][]domain.Instance{
		"ecs":  {{ID: 100, AssetID: "i-a", ModelUID: "ecs"}},
		"disk": {{ID: 200, AssetID: "d-1", ModelUID: "disk", Attributes: map[string]interface{}{"instance_id": "i-a"}}},
	}
	relRepo := &fakeRelationRepoForDerive{}
	// modelRelRepo 传 nil：模拟装配漏注入关系类型仓储。
	svc := NewRelationService(relRepo, instRepo, nil)

	res, err := svc.DeriveRelations(context.Background(), 3, "ecs_disk", false)
	if err != nil {
		t.Fatalf("依赖缺失属降级而非错误，推导应照常成功: %v", err)
	}
	if len(relRepo.created) != 1 || relRepo.created[0].TenantID != 3 {
		t.Fatalf("降级时仍应写入 1 条且带租户: %+v", relRepo.created)
	}
	if len(res.Rules) != 1 || res.Rules[0].SkippedInvalid != 0 {
		t.Fatalf("降级不得把候选判成 skipped_invalid: %+v", res.Rules)
	}
	if res.Rules[0].Candidate != 1 {
		t.Fatalf("候选数应回填为 1，实际 %+v", res.Rules[0])
	}
}
