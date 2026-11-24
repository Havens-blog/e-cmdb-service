package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// 回归背景（实测 2026-11-18）：POST /api/v1/cmdb/instance-relations 只校验端点实例
// 是否存在，不校验两端模型是否符合关系类型声明。用 vpc_subnet（声明 vpc → subnet）
// 把两个 host 实例连起来仍返回 200 成功，关系的方向与基数语义因此不成立，
// 拓扑与影响分析会据此给出错误结论。
// 实测现存 1304 条实例关系全部符合各自关系类型声明的模型对，因此该校验与既有数据兼容。

var ecsDiskRelationType = domain.ModelRelationType{
	ID:             1,
	UID:            "ecs_disk",
	SourceModelUID: "ecs",
	TargetModelUID: "disk",
}

var vpcSubnetRelationType = domain.ModelRelationType{
	ID:             2,
	UID:            "vpc_subnet",
	SourceModelUID: "vpc",
	TargetModelUID: "subnet",
}

// newPairFixture 造两条同租户的 host 实例，便于验证「模型对不匹配」的场景。
func newPairFixture(modelA, modelB string) (*fakeInstanceRepo, *fakeRelationRepoForDerive) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: modelA}
	instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 3, ModelUID: modelB}
	return instRepo, &fakeRelationRepoForDerive{}
}

func TestRelationCreate_RejectsModelPairMismatch(t *testing.T) {
	instRepo, relRepo := newPairFixture("host", "host")
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(vpcSubnetRelationType))

	_, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "vpc_subnet",
	})

	if !errors.Is(err, errs.ErrRelationModelMismatch) {
		t.Fatalf("err = %v, want ErrRelationModelMismatch", err)
	}
	if !strings.Contains(err.Error(), "declared=vpc->subnet") || !strings.Contains(err.Error(), "actual=host->host") {
		t.Fatalf("错误信息应包含声明与实际模型对: %v", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("模型对不匹配时不应写入关系，实际写入 %d 条", len(relRepo.created))
	}
}

func TestRelationCreate_RejectsReversedModelPair(t *testing.T) {
	// 反向（disk → ecs）同样不成立：方向由关系类型固定。
	instRepo, relRepo := newPairFixture("disk", "ecs")
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	if _, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk",
	}); !errors.Is(err, errs.ErrRelationModelMismatch) {
		t.Fatalf("反向模型对应被拒绝，err = %v", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("反向模型对应不写入关系")
	}
}

func TestRelationCreate_AllowsMatchingModelPair(t *testing.T) {
	instRepo, relRepo := newPairFixture("ecs", "disk")
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	if _, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk",
	}); err != nil {
		t.Fatalf("模型对匹配时应创建成功，err = %v", err)
	}
	if len(relRepo.created) != 1 {
		t.Fatalf("应写入 1 条关系，实际 %d", len(relRepo.created))
	}
}

func TestRelationCreate_RejectsUnknownRelationType(t *testing.T) {
	instRepo, relRepo := newPairFixture("ecs", "disk")
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType))

	if _, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "zz_no_such_type",
	}); !errors.Is(err, errs.ErrRelationNotFound) {
		t.Fatalf("未知关系类型应拒绝，err = %v", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("未知关系类型不应写入关系")
	}
}

func TestRelationCreateBatch_RejectsWhenAnyPairMismatches(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instRepo.instances[1] = domain.Instance{ID: 1, TenantID: 3, ModelUID: "ecs"}
	instRepo.instances[2] = domain.Instance{ID: 2, TenantID: 3, ModelUID: "disk"}
	instRepo.instances[3] = domain.Instance{ID: 3, TenantID: 3, ModelUID: "vpc"}
	instRepo.instances[4] = domain.Instance{ID: 4, TenantID: 3, ModelUID: "host"}
	relRepo := &fakeRelationRepoForDerive{}
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(ecsDiskRelationType, vpcSubnetRelationType))

	_, err := svc.CreateBatch(context.Background(), []domain.InstanceRelation{
		{TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "ecs_disk"},
		{TenantID: 3, SourceInstanceID: 3, TargetInstanceID: 4, RelationTypeUID: "vpc_subnet"},
	})
	if !errors.Is(err, errs.ErrRelationModelMismatch) {
		t.Fatalf("批量中任一模型对不匹配应整体拒绝，err = %v", err)
	}
	if len(relRepo.created) != 0 {
		t.Fatalf("整体拒绝时不应写入任何关系，实际 %d", len(relRepo.created))
	}
}

func TestRelationCreate_PairCheckSkippedWithoutModelRepo(t *testing.T) {
	// modelRelRepo 未注入时退化为只校验端点存在性（兼容旧装配与局部单测）。
	instRepo, relRepo := newPairFixture("host", "host")
	svc := NewRelationService(relRepo, instRepo, nil)

	if _, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "vpc_subnet",
	}); err != nil {
		t.Fatalf("未注入 modelRelRepo 时不应因模型对失败，err = %v", err)
	}
}

// 批量创建整体拒绝时，错误必须能定位到具体条目：带上两端 instance_id。
func TestRelationCreateBatch_MismatchErrorCarriesInstanceIDs(t *testing.T) {
	instRepo, relRepo := newPairFixture("host", "host")
	svc := NewRelationService(relRepo, instRepo, newFakeModelRelRepo(vpcSubnetRelationType))

	_, err := svc.CreateBatch(context.Background(), []domain.InstanceRelation{
		{TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "vpc_subnet"},
	})
	if err == nil {
		t.Fatal("模型对不匹配应拒绝批量创建")
	}
	if !errors.Is(err, errs.ErrRelationModelMismatch) {
		t.Fatalf("错误类型不对：%v", err)
	}
	msg := err.Error()
	for _, want := range []string{"source_instance_id=1", "target_instance_id=2", "declared=vpc->subnet"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误信息缺少 %q：%s", want, msg)
		}
	}
}

// instanceRepo 缺失时不能拿空 models 表硬校验模型对（否则一切写入都会被误拒）。
func TestRelationCreate_PairCheckSkippedWithoutInstanceRepo(t *testing.T) {
	_, relRepo := newPairFixture("host", "host")
	svc := NewRelationService(relRepo, nil, newFakeModelRelRepo(vpcSubnetRelationType))

	if _, err := svc.Create(context.Background(), domain.InstanceRelation{
		TenantID: 3, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "vpc_subnet",
	}); err != nil {
		t.Fatalf("未注入 instanceRepo 时不应拒绝写入，err = %v", err)
	}
	if len(relRepo.created) != 1 {
		t.Fatalf("应写入 1 条关系，实际 %d", len(relRepo.created))
	}
}
