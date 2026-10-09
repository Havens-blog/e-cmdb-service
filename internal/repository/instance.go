package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/mongo"
)

// InstanceRepository 资产实例仓储接口
type InstanceRepository interface {
	Create(ctx context.Context, instance domain.Instance) (int64, error)
	CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error)
	Update(ctx context.Context, instance domain.Instance) error
	GetByID(ctx context.Context, id int64) (domain.Instance, error)
	GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error)
	List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, error)
	ListByIDs(ctx context.Context, ids []int64) ([]domain.Instance, error)
	Count(ctx context.Context, filter domain.InstanceFilter) (int64, error)
	Delete(ctx context.Context, id int64) error
	DeleteByAccountID(ctx context.Context, accountID int64) error
	Upsert(ctx context.Context, instance domain.Instance) error
	// ListUnbound 查询未绑定到服务树的资产
	ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]domain.Instance, error)
	// CountUnbound 统计未绑定资产数量
	CountUnbound(ctx context.Context, tenantID int64) (int64, error)
	// AggregateStatsByIDs 根据资源ID列表聚合统计（高性能）
	AggregateStatsByIDs(ctx context.Context, ids []int64) (*dao.AssetStatsResult, error)
	// AggregateAllStats 聚合统计全部资产
	AggregateAllStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error)
	// AggregateUnboundStats 聚合统计未绑定资产
	AggregateUnboundStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error)
}

type instanceRepository struct {
	dao dao.InstanceDAO
}

// NewInstanceRepository 创建实例仓储
func NewInstanceRepository(dao dao.InstanceDAO) InstanceRepository {
	return &instanceRepository{dao: dao}
}

func (r *instanceRepository) Create(ctx context.Context, instance domain.Instance) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(instance))
}

func (r *instanceRepository) CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error) {
	daoInstances := make([]dao.Instance, len(instances))
	for i, inst := range instances {
		daoInstances[i] = r.toDAO(inst)
	}
	return r.dao.CreateBatch(ctx, daoInstances)
}

func (r *instanceRepository) Update(ctx context.Context, instance domain.Instance) error {
	return r.dao.Update(ctx, r.toDAO(instance))
}

func (r *instanceRepository) GetByID(ctx context.Context, id int64) (domain.Instance, error) {
	daoInstance, err := r.dao.GetByID(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.Instance{}, nil
		}
		return domain.Instance{}, err
	}
	return r.toDomain(daoInstance), nil
}

func (r *instanceRepository) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error) {
	daoInstance, err := r.dao.GetByAssetID(ctx, tenantID, modelUID, assetID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.Instance{}, nil
		}
		return domain.Instance{}, err
	}
	return r.toDomain(daoInstance), nil
}

func (r *instanceRepository) ListByIDs(ctx context.Context, ids []int64) ([]domain.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	daoInstances, err := r.dao.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	instances := make([]domain.Instance, len(daoInstances))
	for i, daoInst := range daoInstances {
		instances[i] = r.toDomain(daoInst)
	}
	return instances, nil
}

func (r *instanceRepository) List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, error) {
	daoInstances, err := r.dao.List(ctx, r.toDAOFilter(filter))
	if err != nil {
		return nil, err
	}

	instances := make([]domain.Instance, len(daoInstances))
	for i, daoInst := range daoInstances {
		instances[i] = r.toDomain(daoInst)
	}
	return instances, nil
}

func (r *instanceRepository) Count(ctx context.Context, filter domain.InstanceFilter) (int64, error) {
	return r.dao.Count(ctx, r.toDAOFilter(filter))
}

func (r *instanceRepository) Delete(ctx context.Context, id int64) error {
	return r.dao.Delete(ctx, id)
}

func (r *instanceRepository) DeleteByAccountID(ctx context.Context, accountID int64) error {
	return r.dao.DeleteByAccountID(ctx, accountID)
}

func (r *instanceRepository) Upsert(ctx context.Context, instance domain.Instance) error {
	return r.dao.Upsert(ctx, r.toDAO(instance))
}

func (r *instanceRepository) ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]domain.Instance, error) {
	daoInstances, err := r.dao.ListUnbound(ctx, tenantID, offset, limit)
	if err != nil {
		return nil, err
	}
	instances := make([]domain.Instance, len(daoInstances))
	for i, daoInst := range daoInstances {
		instances[i] = r.toDomain(daoInst)
	}
	return instances, nil
}

func (r *instanceRepository) CountUnbound(ctx context.Context, tenantID int64) (int64, error) {
	return r.dao.CountUnbound(ctx, tenantID)
}

func (r *instanceRepository) AggregateStatsByIDs(ctx context.Context, ids []int64) (*dao.AssetStatsResult, error) {
	return r.dao.AggregateStatsByIDs(ctx, ids)
}

func (r *instanceRepository) AggregateAllStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return r.dao.AggregateAllStats(ctx, tenantID)
}

func (r *instanceRepository) AggregateUnboundStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return r.dao.AggregateUnboundStats(ctx, tenantID)
}

func (r *instanceRepository) toDAO(instance domain.Instance) dao.Instance {
	return dao.Instance{
		ID:         instance.ID,
		ModelUID:   instance.ModelUID,
		AssetID:    instance.AssetID,
		AssetName:  instance.AssetName,
		TenantID:   instance.TenantID,
		AccountID:  instance.AccountID,
		Attributes: instance.Attributes,
	}
}

func (r *instanceRepository) toDomain(daoInstance dao.Instance) domain.Instance {
	return domain.Instance{
		ID:         daoInstance.ID,
		ModelUID:   daoInstance.ModelUID,
		AssetID:    daoInstance.AssetID,
		AssetName:  daoInstance.AssetName,
		TenantID:   daoInstance.TenantID,
		AccountID:  daoInstance.AccountID,
		Attributes: daoInstance.Attributes,
		CreateTime: time.UnixMilli(daoInstance.Ctime),
		UpdateTime: time.UnixMilli(daoInstance.Utime),
	}
}

func (r *instanceRepository) toDAOFilter(filter domain.InstanceFilter) dao.InstanceFilter {
	return dao.InstanceFilter{
		ModelUID:   filter.ModelUID,
		TenantID:   filter.TenantID,
		AccountID:  filter.AccountID,
		AssetName:  filter.AssetName,
		Attributes: filter.Attributes,
		Offset:     filter.Offset,
		Limit:      filter.Limit,
	}
}
