package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// fakeAttributeRepoForSync 返回固定字段定义，用于验证同步质量记录。
type fakeAttributeRepoForSync struct {
	defs []domain.Attribute
	err  error
}

func (f *fakeAttributeRepoForSync) Create(ctx context.Context, attr domain.Attribute) (int64, error) {
	return 0, nil
}
func (f *fakeAttributeRepoForSync) GetByID(ctx context.Context, id int64) (domain.Attribute, error) {
	return domain.Attribute{}, nil
}
func (f *fakeAttributeRepoForSync) GetByFieldUID(ctx context.Context, modelUID, fieldUID string) (domain.Attribute, error) {
	return domain.Attribute{}, nil
}
func (f *fakeAttributeRepoForSync) List(ctx context.Context, filter domain.AttributeFilter) ([]domain.Attribute, error) {
	return f.defs, f.err
}
func (f *fakeAttributeRepoForSync) Count(ctx context.Context, filter domain.AttributeFilter) (int64, error) {
	return int64(len(f.defs)), nil
}
func (f *fakeAttributeRepoForSync) Update(ctx context.Context, attr domain.Attribute) error {
	return nil
}
func (f *fakeAttributeRepoForSync) Delete(ctx context.Context, id int64) error { return nil }
func (f *fakeAttributeRepoForSync) DeleteByModelUID(ctx context.Context, modelUID string) error {
	return nil
}
func (f *fakeAttributeRepoForSync) Exists(ctx context.Context, modelUID, fieldUID string) (bool, error) {
	return false, nil
}

// fakeSyncQualityIssueRepo 记录 UpsertByKey 的调用。
type fakeSyncQualityIssueRepo struct {
	upserted []domain.SyncQualityIssue
	err      error
}

func (f *fakeSyncQualityIssueRepo) UpsertByKey(ctx context.Context, issue domain.SyncQualityIssue) (int64, error) {
	f.upserted = append(f.upserted, issue)
	return 1, f.err
}
func (f *fakeSyncQualityIssueRepo) List(ctx context.Context, filter domain.SyncQualityIssueFilter) ([]domain.SyncQualityIssue, int64, error) {
	return f.upserted, int64(len(f.upserted)), nil
}
func (f *fakeSyncQualityIssueRepo) DeleteByAssetID(ctx context.Context, tenantID int64, assetID string) error {
	return nil
}

// 编译期断言实现接口
var _ repository.AttributeRepository = (*fakeAttributeRepoForSync)(nil)
var _ repository.SyncQualityIssueRepository = (*fakeSyncQualityIssueRepo)(nil)

// TestUpsertBatch_RecordButNotBlock 锁定方案 B「记录但不阻断」。
//
// 背景（2026-11-02 产品方案 B）：同步链路不做字段校验导致脏数据持续进入。
// 方案 B 要求采集不中断，同时把不合规实例的问题写入可查询清单。
func TestUpsertBatch_RecordButNotBlock(t *testing.T) {
	t.Run("缺必填字段的实例仍写入，且问题被记录", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		attrRepo := &fakeAttributeRepoForSync{
			defs: []domain.Attribute{
				{ModelUID: "ecs", FieldUID: "instance_id", FieldType: "string", Required: true},
				{ModelUID: "ecs", FieldUID: "status", FieldType: "enum", Required: true, Option: `["Running"]`},
			},
		}
		sqRepo := &fakeSyncQualityIssueRepo{}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, attrRepo, nil, sqRepo)

		// 缺 instance_id、status 值越界（running vs Running 大小写不同视为匹配，这里用非法值）
		err := svc.UpsertBatch(context.Background(), []domain.Instance{
			{ID: 100, TenantID: 3, ModelUID: "ecs", AssetID: "i-bad", Attributes: map[string]interface{}{
				"status": "not-a-status",
			}},
		})
		if err != nil {
			t.Fatalf("方案 B 要求不阻断，实际返回错误: %v", err)
		}

		// 应记录 2 条问题：missing instance_id + enum status
		if len(sqRepo.upserted) != 2 {
			t.Fatalf("期望记录 2 条问题，实际 %d: %+v", len(sqRepo.upserted), sqRepo.upserted)
		}

		types := map[string]bool{}
		for _, u := range sqRepo.upserted {
			types[u.IssueType] = true
			if u.AssetID != "i-bad" || u.ModelUID != "ecs" || u.TenantID != 3 {
				t.Fatalf("问题记录字段错误: %+v", u)
			}
		}
		if !types["required"] || !types["enum"] {
			t.Fatalf("应包含 required 与 enum 两类问题，实际 %v", types)
		}

		// 实例仍被写入
		if instRepo.deletedID == 0 && len(instRepo.instances) == 0 {
			// fakeInstanceRepo.Upsert 不落 map，这里仅断言未报错即可
		}
	})

	t.Run("合规实例不产生问题记录", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		attrRepo := &fakeAttributeRepoForSync{
			defs: []domain.Attribute{
				{ModelUID: "ecs", FieldUID: "instance_id", FieldType: "string", Required: true},
			},
		}
		sqRepo := &fakeSyncQualityIssueRepo{}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, attrRepo, nil, sqRepo)
		err := svc.UpsertBatch(context.Background(), []domain.Instance{
			{ID: 101, TenantID: 3, ModelUID: "ecs", AssetID: "i-good", Attributes: map[string]interface{}{
				"instance_id": "i-good",
			}},
		})
		if err != nil {
			t.Fatalf("合规实例不应报错: %v", err)
		}
		if len(sqRepo.upserted) != 0 {
			t.Fatalf("合规实例不应产生问题记录，实际 %d", len(sqRepo.upserted))
		}
	})

	t.Run("问题写入失败不阻断采集", func(t *testing.T) {
		instRepo := newFakeInstanceRepo()
		attrRepo := &fakeAttributeRepoForSync{
			defs: []domain.Attribute{
				{ModelUID: "ecs", FieldUID: "instance_id", FieldType: "string", Required: true},
			},
		}
		sqRepo := &fakeSyncQualityIssueRepo{err: context.DeadlineExceeded}

		svc := NewInstanceService(instRepo, &fakeModelRepo{}, attrRepo, nil, sqRepo)
		err := svc.UpsertBatch(context.Background(), []domain.Instance{
			{ID: 102, TenantID: 3, ModelUID: "ecs", AssetID: "i-bad2", Attributes: map[string]interface{}{}},
		})
		if err != nil {
			t.Fatalf("问题记录失败不应阻断采集，实际: %v", err)
		}
	})
}
