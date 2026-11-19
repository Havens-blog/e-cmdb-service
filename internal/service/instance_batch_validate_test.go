package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// batchValidateRepo 在 fakeInstanceRepo 之上补两个能力：
//   - GetByAssetID 可配置「库里已存在」
//   - CreateBatch 记录是否真的落库
type batchValidateRepo struct {
	*fakeInstanceRepo
	existing map[string]bool // key: modelUID|assetID
	created  int
}

func newBatchValidateRepo() *batchValidateRepo {
	return &batchValidateRepo{fakeInstanceRepo: newFakeInstanceRepo(), existing: map[string]bool{}}
}

func (r *batchValidateRepo) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error) {
	if r.existing[modelUID+"|"+assetID] {
		return domain.Instance{ID: 777, TenantID: tenantID, ModelUID: modelUID, AssetID: assetID}, nil
	}
	return domain.Instance{}, nil
}

func (r *batchValidateRepo) CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error) {
	r.created = len(instances)
	return int64(len(instances)), nil
}

// ecsRequiredDefs 复现线上 ecs 的必填字段：provider / cloud_account_id /
// instance_id / instance_name / status / instance_type（2026-11-19 实测的错误文案来源）。
func ecsRequiredDefs() []domain.Attribute {
	names := []struct{ uid, name string }{
		{"provider", "云厂商"},
		{"cloud_account_id", "云账号ID"},
		{"instance_id", "实例ID"},
		{"instance_name", "实例名称"},
		{"status", "状态"},
		{"instance_type", "实例规格"},
	}
	defs := make([]domain.Attribute, 0, len(names))
	for _, n := range names {
		defs = append(defs, domain.Attribute{
			ModelUID: "ecs", FieldUID: n.uid, FieldName: n.name, FieldType: "string", Required: true, Editable: true,
		})
	}
	return defs
}

func newBatchValidateService(repo *batchValidateRepo, model domain.Model, defs []domain.Attribute) InstanceService {
	modelRepo := &fakeModelRepo{getByUID: model}
	attrRepo := &fakeAttributeRepoForSync{defs: defs}
	return NewInstanceService(repo, modelRepo, attrRepo, nil, nil, nil)
}

func fullEcsInstance(assetID string) domain.Instance {
	return domain.Instance{
		ModelUID:  "ecs",
		AssetID:   assetID,
		TenantID:  3,
		AssetName: assetID + "-name",
		Attributes: map[string]interface{}{
			"provider": "aliyun", "cloud_account_id": 1, "instance_id": assetID,
			"instance_name": assetID + "-inst", "status": "running", "instance_type": "ecs.g6.large",
		},
	}
}

// TestCreateBatch_RejectsMissingRequiredAttributes 回归 2026-11-19 的实测缺陷：
// POST /instances/batch 传 attributes:{} 的 ecs 条目返回 code=0 并落库，
// 而单条 POST /instances 同样入参被拒（缺少 6 个必填字段）。批量必须同样拒绝。
func TestCreateBatch_RejectsMissingRequiredAttributes(t *testing.T) {
	repo := newBatchValidateRepo()
	svc := newBatchValidateService(repo, domain.Model{ID: 1, UID: "ecs"}, ecsRequiredDefs())

	_, err := svc.CreateBatch(context.Background(), []domain.Instance{
		{ModelUID: "ecs", AssetID: "zz-batch-bare", TenantID: 3, Attributes: map[string]interface{}{}},
	})
	if err == nil {
		t.Fatal("批量创建缺必填字段应被拒绝，实际通过")
	}
	if !errors.Is(err, errs.ErrInvalidAttributes) {
		t.Fatalf("错误应可判定为 ErrInvalidAttributes，实际 %v", err)
	}
	if repo.created != 0 {
		t.Fatalf("校验失败时不得落库，实际写入 %d 条", repo.created)
	}

	var itemErr *BatchItemError
	if !errors.As(err, &itemErr) {
		t.Fatalf("批量错误应带条目定位信息，实际 %T: %v", err, err)
	}
	if itemErr.Index != 0 || itemErr.AssetID != "zz-batch-bare" || itemErr.ModelUID != "ecs" {
		t.Fatalf("条目定位信息错误: %+v", itemErr)
	}
	// 文案要能指向具体字段，便于前端直接展示
	for _, uid := range []string{"provider", "status", "instance_type"} {
		if !strings.Contains(err.Error(), uid) {
			t.Fatalf("错误文案应包含缺失字段 %s，实际: %v", uid, err)
		}
	}
}

// TestCreateBatch_RejectsUnknownModel 批量入口此前完全不查模型存在性。
func TestCreateBatch_RejectsUnknownModel(t *testing.T) {
	repo := newBatchValidateRepo()
	// fakeModelRepo 返回零值模型 = 模型不存在
	svc := newBatchValidateService(repo, domain.Model{}, ecsRequiredDefs())

	_, err := svc.CreateBatch(context.Background(), []domain.Instance{fullEcsInstance("zz-batch-nomodel")})
	if !errors.Is(err, errs.ErrModelNotFound) {
		t.Fatalf("未知模型应返回 ErrModelNotFound，实际 %v", err)
	}
	if repo.created != 0 {
		t.Fatalf("模型不存在时不得落库，实际写入 %d 条", repo.created)
	}
}

// TestCreateBatch_RejectsDuplicateAgainstDB 库中已有同 (tenant, model, asset_id)。
func TestCreateBatch_RejectsDuplicateAgainstDB(t *testing.T) {
	repo := newBatchValidateRepo()
	repo.existing["ecs|zz-batch-dup"] = true
	svc := newBatchValidateService(repo, domain.Model{ID: 1, UID: "ecs"}, ecsRequiredDefs())

	_, err := svc.CreateBatch(context.Background(), []domain.Instance{fullEcsInstance("zz-batch-dup")})
	if !errors.Is(err, errs.ErrInstanceExists) {
		t.Fatalf("已存在资产应返回 ErrInstanceExists，实际 %v", err)
	}
	if repo.created != 0 {
		t.Fatalf("重复时不得落库，实际写入 %d 条", repo.created)
	}
}

// TestCreateBatch_RejectsDuplicateWithinBatch 同批次内两条相同 (model, asset_id)
// 会撞唯一索引 E11000，且调用方拿不到「第几条」——必须在进入 repo 前拦下。
func TestCreateBatch_RejectsDuplicateWithinBatch(t *testing.T) {
	repo := newBatchValidateRepo()
	svc := newBatchValidateService(repo, domain.Model{ID: 1, UID: "ecs"}, ecsRequiredDefs())

	_, err := svc.CreateBatch(context.Background(), []domain.Instance{
		fullEcsInstance("zz-batch-same"),
		fullEcsInstance("zz-batch-same"),
	})
	if !errors.Is(err, errs.ErrInstanceExists) {
		t.Fatalf("批内重复应返回 ErrInstanceExists，实际 %v", err)
	}
	var itemErr *BatchItemError
	if !errors.As(err, &itemErr) || itemErr.Index != 1 {
		t.Fatalf("应指出第 2 条（下标 1）重复，实际 %+v", itemErr)
	}
	if !strings.Contains(err.Error(), "同一批次内重复出现") {
		t.Fatalf("批内重复文案应说明原因，实际 %v", err)
	}
	if repo.created != 0 {
		t.Fatalf("批内重复时不得落库，实际写入 %d 条", repo.created)
	}
}

// TestCreateBatch_AllowsValidItems 合法批量仍应成功（不能因加严校验而阻断正常路径）。
func TestCreateBatch_AllowsValidItems(t *testing.T) {
	repo := newBatchValidateRepo()
	svc := newBatchValidateService(repo, domain.Model{ID: 1, UID: "ecs"}, ecsRequiredDefs())

	n, err := svc.CreateBatch(context.Background(), []domain.Instance{
		fullEcsInstance("zz-batch-ok-1"),
		fullEcsInstance("zz-batch-ok-2"),
	})
	if err != nil {
		t.Fatalf("合法批量不应报错，实际 %v", err)
	}
	if n != 2 || repo.created != 2 {
		t.Fatalf("应写入 2 条，实际 n=%d created=%d", n, repo.created)
	}
}

// TestCreateBatch_EmptyNoop 空批次保持原有「no-op 返回 0」语义。
func TestCreateBatch_EmptyNoop(t *testing.T) {
	repo := newBatchValidateRepo()
	svc := newBatchValidateService(repo, domain.Model{ID: 1, UID: "ecs"}, ecsRequiredDefs())
	n, err := svc.CreateBatch(context.Background(), nil)
	if err != nil || n != 0 {
		t.Fatalf("空批次应返回 (0, nil)，实际 (%d, %v)", n, err)
	}
}

// TestCreate_SingleAndBatchShareValidation 单条与批量对同一非法入参给出同类错误，
// 防止两者口径再次分叉。
func TestCreate_SingleAndBatchShareValidation(t *testing.T) {
	repo := newBatchValidateRepo()
	svc := newBatchValidateService(repo, domain.Model{ID: 1, UID: "ecs"}, ecsRequiredDefs())
	bare := domain.Instance{ModelUID: "ecs", AssetID: "zz-consistency", TenantID: 3, Attributes: map[string]interface{}{}}

	_, singleErr := svc.Create(context.Background(), bare)
	_, batchErr := svc.CreateBatch(context.Background(), []domain.Instance{bare})

	if singleErr == nil || batchErr == nil {
		t.Fatalf("单条与批量都应拒绝缺必填字段，实际 single=%v batch=%v", singleErr, batchErr)
	}
	if !errors.Is(singleErr, errs.ErrInvalidAttributes) || !errors.Is(batchErr, errs.ErrInvalidAttributes) {
		t.Fatalf("两者都应判定为 ErrInvalidAttributes，实际 single=%v batch=%v", singleErr, batchErr)
	}

	var itemErr *BatchItemError
	if !errors.As(batchErr, &itemErr) {
		t.Fatalf("批量错误必须可定位到条目，实际 %T", batchErr)
	}
}
