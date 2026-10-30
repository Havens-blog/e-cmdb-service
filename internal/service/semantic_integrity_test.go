package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// fakeModelRepo 是 ModelRepository 的内存实现，用于语义加固的 service 单测。
type fakeModelRepo struct {
	getByUID    domain.Model
	getByUIDErr error
	exists      bool
	existsErr   error
	deletedUID  string
	deleteErr   error
}

func (f *fakeModelRepo) Create(ctx context.Context, model domain.Model) (int64, error) {
	return 0, nil
}

func (f *fakeModelRepo) GetByUID(ctx context.Context, uid string) (domain.Model, error) {
	if f.getByUIDErr != nil {
		return domain.Model{}, f.getByUIDErr
	}
	return f.getByUID, nil
}

func (f *fakeModelRepo) GetByID(ctx context.Context, id int64) (domain.Model, error) {
	return domain.Model{}, nil
}

func (f *fakeModelRepo) List(ctx context.Context, filter domain.ModelFilter) ([]domain.Model, error) {
	return nil, nil
}

func (f *fakeModelRepo) Count(ctx context.Context, filter domain.ModelFilter) (int64, error) {
	return 0, nil
}

func (f *fakeModelRepo) Update(ctx context.Context, model domain.Model) error {
	return nil
}

func (f *fakeModelRepo) Delete(ctx context.Context, uid string) error {
	f.deletedUID = uid
	return f.deleteErr
}

func (f *fakeModelRepo) Exists(ctx context.Context, uid string) (bool, error) {
	return f.exists, f.existsErr
}

// semanticInstanceRepo 是 InstanceRepository 的内存实现，可配置 Count/GetByAssetID。
type semanticInstanceRepo struct {
	count        int64
	countErr     error
	getByAssetID domain.Instance
	createCalled bool
	created      domain.Instance
	createErr    error
}

func (f *semanticInstanceRepo) Create(ctx context.Context, instance domain.Instance) (int64, error) {
	f.createCalled = true
	f.created = instance
	return instance.ID, f.createErr
}

func (f *semanticInstanceRepo) CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error) {
	return 0, nil
}

func (f *semanticInstanceRepo) Update(ctx context.Context, tenantID int64, instance domain.Instance) error {
	return nil
}

func (f *semanticInstanceRepo) GetByID(ctx context.Context, tenantID int64, id int64) (domain.Instance, error) {
	return domain.Instance{}, nil
}

func (f *semanticInstanceRepo) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error) {
	return f.getByAssetID, nil
}

func (f *semanticInstanceRepo) List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, error) {
	return nil, nil
}

func (f *semanticInstanceRepo) ListByIDs(ctx context.Context, ids []int64, tenantID int64) ([]domain.Instance, error) {
	return nil, nil
}

func (f *semanticInstanceRepo) Count(ctx context.Context, filter domain.InstanceFilter) (int64, error) {
	return f.count, f.countErr
}

// CountByModelUID 桩实现：数据质量服务端的按模型计数（测试中不使用）
func (f *semanticInstanceRepo) CountByModelUID(ctx context.Context, tenantID int64) (map[string]int64, error) {
	return map[string]int64{}, nil
}

// CountMissingAttribute 桩实现：数据质量服务端的缺失字段计数（测试中不使用）
func (f *semanticInstanceRepo) CountMissingAttribute(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	return 0, nil
}
func (f *semanticInstanceRepo) Delete(ctx context.Context, tenantID int64, id int64) error {
	return nil
}

func (f *semanticInstanceRepo) DeleteByAccountID(ctx context.Context, accountID int64) error {
	return nil
}

func (f *semanticInstanceRepo) Upsert(ctx context.Context, instance domain.Instance) error {
	return nil
}

func (f *semanticInstanceRepo) ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]domain.Instance, error) {
	return nil, nil
}

func (f *semanticInstanceRepo) CountUnbound(ctx context.Context, tenantID int64) (int64, error) {
	return 0, nil
}

func (f *semanticInstanceRepo) AggregateStatsByIDs(ctx context.Context, ids []int64, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}

func (f *semanticInstanceRepo) AggregateAllStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}

func (f *semanticInstanceRepo) AggregateUnboundStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}

// semanticRelationRepo 是 InstanceRelationRepository 的内存实现，Exists 按租户配置结果。
type semanticRelationRepo struct {
	existsTenant int64
	existsResult bool
	createID     int64
	createCalled bool
	createErr    error
}

func (f *semanticRelationRepo) Create(ctx context.Context, relation domain.InstanceRelation) (int64, error) {
	f.createCalled = true
	return f.createID, f.createErr
}

func (f *semanticRelationRepo) CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error) {
	return 0, nil
}

func (f *semanticRelationRepo) GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error) {
	return domain.InstanceRelation{}, nil
}

func (f *semanticRelationRepo) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error) {
	return nil, nil
}

func (f *semanticRelationRepo) Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	return 0, nil
}

func (f *semanticRelationRepo) Delete(ctx context.Context, tenantID int64, id int64) error {
	return nil
}

func (f *semanticRelationRepo) DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error {
	return nil
}

func (f *semanticRelationRepo) Exists(ctx context.Context, tenantID int64, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	return tenantID == f.existsTenant && f.existsResult, nil
}

func TestInstanceCreate_ModelNotFound(t *testing.T) {
	svc := NewInstanceService(
		&semanticInstanceRepo{},
		&fakeModelRepo{getByUID: domain.Model{}},
	)

	_, err := svc.Create(context.Background(), domain.Instance{
		ModelUID: "missing_model",
		AssetID:  "a1",
		TenantID: 1,
	})
	if !errors.Is(err, errs.ErrModelNotFound) {
		t.Fatalf("Create() error = %v, want ErrModelNotFound", err)
	}
}

func TestModelDelete_HasInstances(t *testing.T) {
	svc := NewModelService(
		&fakeModelRepo{exists: true},
		&semanticInstanceRepo{count: 3},
	)

	err := svc.Delete(context.Background(), "m1")
	if !errors.Is(err, errs.ErrModelHasInstances) {
		t.Fatalf("Delete() error = %v, want ErrModelHasInstances", err)
	}
}

func TestModelDelete_NoInstances(t *testing.T) {
	modelRepo := &fakeModelRepo{exists: true}
	svc := NewModelService(modelRepo, &semanticInstanceRepo{count: 0})

	if err := svc.Delete(context.Background(), "m1"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if modelRepo.deletedUID != "m1" {
		t.Fatalf("deleted uid = %q, want m1", modelRepo.deletedUID)
	}
}

func TestRelationCreate_SameTenantDuplicate(t *testing.T) {
	repo := &semanticRelationRepo{existsTenant: 1, existsResult: true}
	svc := NewRelationService(repo)

	_, err := svc.Create(context.Background(), domain.InstanceRelation{
		SourceInstanceID: 1,
		TargetInstanceID: 2,
		RelationTypeUID:  "depends_on",
		TenantID:         1,
	})
	if !errors.Is(err, errs.ErrRelationExists) {
		t.Fatalf("Create() error = %v, want ErrRelationExists", err)
	}
}

func TestRelationCreate_CrossTenantNoConflict(t *testing.T) {
	repo := &semanticRelationRepo{existsTenant: 1, existsResult: true, createID: 99}
	svc := NewRelationService(repo)

	id, err := svc.Create(context.Background(), domain.InstanceRelation{
		SourceInstanceID: 1,
		TargetInstanceID: 2,
		RelationTypeUID:  "depends_on",
		TenantID:         2,
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id != 99 {
		t.Fatalf("Create() id = %d, want 99", id)
	}
	if !repo.createCalled {
		t.Fatalf("repo.Create should be called for cross-tenant relation")
	}
}

// CountOrphanInstances 桩实现：数据质量孤儿统计（测试中不使用）。
func (f *semanticInstanceRepo) CountOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string) (int64, error) {
	return 0, nil
}

// ListOrphanInstances 桩实现：数据质量孤儿明细（测试中不使用）。
func (f *semanticInstanceRepo) ListOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string, offset, limit int64) ([]repository.OrphanInstanceView, int64, error) {
	return nil, 0, nil
}

// CountMissingFieldGroup 桩实现：数据质量缺失字段分组计数（测试中不使用）。
func (f *semanticInstanceRepo) CountMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	return 0, nil
}

// ListMissingFieldGroup 桩实现：数据质量缺失字段分组明细（测试中不使用）。
func (f *semanticInstanceRepo) ListMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string, offset, limit int64) ([]repository.MissingFieldInstanceView, error) {
	return nil, nil
}
