package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// --- instance repo fake：只验证租户参数被正确传递，并按租户隔离返回 ---

type fakeInstanceRepo struct {
	instances     map[int64]domain.Instance
	gotTenant     int64
	gotID         int64
	updatedTenant int64
	updated       domain.Instance
	deletedTenant int64
	deletedID     int64
}

func newFakeInstanceRepo() *fakeInstanceRepo {
	return &fakeInstanceRepo{instances: map[int64]domain.Instance{}}
}

func (f *fakeInstanceRepo) Create(ctx context.Context, instance domain.Instance) (int64, error) {
	return 0, nil
}
func (f *fakeInstanceRepo) CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error) {
	return 0, nil
}
func (f *fakeInstanceRepo) Update(ctx context.Context, tenantID int64, instance domain.Instance) error {
	f.updatedTenant = tenantID
	f.updated = instance
	return nil
}
func (f *fakeInstanceRepo) GetByID(ctx context.Context, tenantID int64, id int64) (domain.Instance, error) {
	f.gotTenant = tenantID
	f.gotID = id
	inst, ok := f.instances[id]
	if !ok || inst.TenantID != tenantID {
		return domain.Instance{}, nil
	}
	return inst, nil
}
func (f *fakeInstanceRepo) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error) {
	return domain.Instance{}, nil
}
func (f *fakeInstanceRepo) List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepo) ListByIDs(ctx context.Context, ids []int64) ([]domain.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepo) Count(ctx context.Context, filter domain.InstanceFilter) (int64, error) {
	return 0, nil
}
func (f *fakeInstanceRepo) Delete(ctx context.Context, tenantID int64, id int64) error {
	f.deletedTenant = tenantID
	f.deletedID = id
	return nil
}
func (f *fakeInstanceRepo) DeleteByAccountID(ctx context.Context, accountID int64) error { return nil }
func (f *fakeInstanceRepo) Upsert(ctx context.Context, instance domain.Instance) error {
	return nil
}
func (f *fakeInstanceRepo) ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]domain.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepo) CountUnbound(ctx context.Context, tenantID int64) (int64, error) {
	return 0, nil
}
func (f *fakeInstanceRepo) AggregateStatsByIDs(ctx context.Context, ids []int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}
func (f *fakeInstanceRepo) AggregateAllStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}
func (f *fakeInstanceRepo) AggregateUnboundStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}

func TestInstanceServiceGetByID_TenantMismatch(t *testing.T) {
	repo := newFakeInstanceRepo()
	repo.instances[1] = domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a"}
	svc := NewInstanceService(repo, &fakeModelRepo{})

	_, err := svc.GetByID(context.Background(), 200, 1)
	if !errors.Is(err, errs.ErrInstanceNotFound) {
		t.Fatalf("GetByID error = %v, want ErrInstanceNotFound", err)
	}
	if repo.gotTenant != 200 || repo.gotID != 1 {
		t.Fatalf("GetByID tenant/id = %d/%d, want 200/1", repo.gotTenant, repo.gotID)
	}
}

func TestInstanceServiceGetByID_TenantMatch(t *testing.T) {
	repo := newFakeInstanceRepo()
	repo.instances[1] = domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a"}
	svc := NewInstanceService(repo, &fakeModelRepo{})

	got, err := svc.GetByID(context.Background(), 100, 1)
	if err != nil {
		t.Fatalf("GetByID error = %v", err)
	}
	if got.ID != 1 {
		t.Fatalf("got.ID = %d, want 1", got.ID)
	}
}

func TestInstanceServiceDelete_TenantMismatch(t *testing.T) {
	repo := newFakeInstanceRepo()
	repo.instances[1] = domain.Instance{ID: 1, TenantID: 100}
	svc := NewInstanceService(repo, &fakeModelRepo{})

	err := svc.Delete(context.Background(), 200, 1)
	if !errors.Is(err, errs.ErrInstanceNotFound) {
		t.Fatalf("Delete error = %v, want ErrInstanceNotFound", err)
	}
	if repo.deletedID != 0 {
		t.Fatalf("repo.Delete should not be called on tenant mismatch")
	}
}

func TestInstanceServiceDelete_TenantMatch(t *testing.T) {
	repo := newFakeInstanceRepo()
	repo.instances[1] = domain.Instance{ID: 1, TenantID: 100}
	svc := NewInstanceService(repo, &fakeModelRepo{})

	if err := svc.Delete(context.Background(), 100, 1); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if repo.deletedTenant != 100 || repo.deletedID != 1 {
		t.Fatalf("Delete tenant/id = %d/%d, want 100/1", repo.deletedTenant, repo.deletedID)
	}
}

func TestInstanceServiceUpdate_ForwardsTenant(t *testing.T) {
	repo := newFakeInstanceRepo()
	svc := NewInstanceService(repo, &fakeModelRepo{})

	inst := domain.Instance{ID: 7, ModelUID: "m", AssetID: "a", TenantID: 100}
	if err := svc.Update(context.Background(), 100, inst); err != nil {
		t.Fatalf("Update error = %v", err)
	}
	if repo.updatedTenant != 100 || repo.updated.ID != 7 {
		t.Fatalf("Update tenant/id = %d/%d, want 100/7", repo.updatedTenant, repo.updated.ID)
	}
}

// --- relation repo fake ---

type fakeRelationRepo struct {
	relations           map[int64]domain.InstanceRelation
	gotTenant           int64
	gotID               int64
	deletedTenant       int64
	deletedID           int64
	deleteByInstanceTen int64
	deleteByInstanceID  int64
}

func newFakeRelationRepo() *fakeRelationRepo {
	return &fakeRelationRepo{relations: map[int64]domain.InstanceRelation{}}
}

func (f *fakeRelationRepo) Create(ctx context.Context, relation domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepo) CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepo) GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error) {
	f.gotTenant = tenantID
	f.gotID = id
	rel, ok := f.relations[id]
	if !ok || rel.TenantID != tenantID {
		return domain.InstanceRelation{}, nil
	}
	return rel, nil
}
func (f *fakeRelationRepo) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error) {
	return nil, nil
}
func (f *fakeRelationRepo) Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	return 0, nil
}
func (f *fakeRelationRepo) Delete(ctx context.Context, tenantID int64, id int64) error {
	f.deletedTenant = tenantID
	f.deletedID = id
	return nil
}
func (f *fakeRelationRepo) DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error {
	f.deleteByInstanceTen = tenantID
	f.deleteByInstanceID = instanceID
	return nil
}
func (f *fakeRelationRepo) Exists(ctx context.Context, tenantID int64, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	return false, nil
}

func TestRelationServiceGetByID_TenantMismatch(t *testing.T) {
	repo := newFakeRelationRepo()
	repo.relations[10] = domain.InstanceRelation{ID: 10, TenantID: 100}
	svc := NewRelationService(repo)

	_, err := svc.GetByID(context.Background(), 200, 10)
	if !errors.Is(err, errs.ErrRelationNotFound) {
		t.Fatalf("GetByID error = %v, want ErrRelationNotFound", err)
	}
	if repo.gotTenant != 200 || repo.gotID != 10 {
		t.Fatalf("GetByID tenant/id = %d/%d, want 200/10", repo.gotTenant, repo.gotID)
	}
}

func TestRelationServiceDelete_TenantMismatch(t *testing.T) {
	repo := newFakeRelationRepo()
	repo.relations[10] = domain.InstanceRelation{ID: 10, TenantID: 100}
	svc := NewRelationService(repo)

	err := svc.Delete(context.Background(), 200, 10)
	if !errors.Is(err, errs.ErrRelationNotFound) {
		t.Fatalf("Delete error = %v, want ErrRelationNotFound", err)
	}
	if repo.deletedID != 0 {
		t.Fatalf("repo.Delete should not be called on tenant mismatch")
	}
}

func TestRelationServiceDelete_TenantMatch(t *testing.T) {
	repo := newFakeRelationRepo()
	repo.relations[10] = domain.InstanceRelation{ID: 10, TenantID: 100}
	svc := NewRelationService(repo)

	if err := svc.Delete(context.Background(), 100, 10); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if repo.deletedTenant != 100 || repo.deletedID != 10 {
		t.Fatalf("Delete tenant/id = %d/%d, want 100/10", repo.deletedTenant, repo.deletedID)
	}
}

func TestRelationServiceDeleteByInstanceID_ForwardsTenant(t *testing.T) {
	repo := newFakeRelationRepo()
	svc := NewRelationService(repo)

	if err := svc.DeleteByInstanceID(context.Background(), 100, 55); err != nil {
		t.Fatalf("DeleteByInstanceID error = %v", err)
	}
	if repo.deleteByInstanceTen != 100 || repo.deleteByInstanceID != 55 {
		t.Fatalf("DeleteByInstanceID tenant/id = %d/%d, want 100/55", repo.deleteByInstanceTen, repo.deleteByInstanceID)
	}
}
