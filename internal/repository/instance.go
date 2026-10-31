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
	Update(ctx context.Context, tenantID int64, instance domain.Instance) error
	GetByID(ctx context.Context, tenantID int64, id int64) (domain.Instance, error)
	GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error)
	List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, error)
	ListByIDs(ctx context.Context, ids []int64, tenantID int64) ([]domain.Instance, error)
	Count(ctx context.Context, filter domain.InstanceFilter) (int64, error)
	// CountByModelUID 按 model_uid 分组统计租户下实例数量（服务端聚合）
	CountByModelUID(ctx context.Context, tenantID int64) (map[string]int64, error)
	// CountOrphanInstances 统计孤儿实例数量（model_uid 不在模型表）
	CountOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string) (int64, error)
	// ListOrphanInstances 分页拉取孤儿实例
	ListOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string, offset, limit int64) ([]OrphanInstanceView, int64, error)
	// GroupOrphanInstancesByModel 按 model_uid 聚合统计孤儿实例数，按数量降序
	GroupOrphanInstancesByModel(ctx context.Context, tenantID int64, knownModelUIDs []string) ([]OrphanModelGroup, error)
	// CountMissingFieldGroup 统计某 (模型,字段) 组合缺该属性的实例数
	CountMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error)
	// BatchCountMissingFieldGroups 一次聚合算出多个 (模型,字段) 组合的缺失实例数
	BatchCountMissingFieldGroups(ctx context.Context, tenantID int64, fieldsByModel map[string][]string) (map[string]map[string]int64, error)
	// ListMissingFieldGroup 分页拉取某 (模型,字段) 组合缺该属性的实例
	ListMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string, offset, limit int64) ([]MissingFieldInstanceView, error)
	// CountMissingAttribute 统计某模型下缺少指定属性的实例数（服务端过滤）
	CountMissingAttribute(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error)
	Delete(ctx context.Context, tenantID int64, id int64) error
	DeleteByAccountID(ctx context.Context, accountID int64) error
	Upsert(ctx context.Context, instance domain.Instance) error
	// ListUnbound 查询未绑定到服务树的资产
	ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]domain.Instance, error)
	// CountUnbound 统计未绑定资产数量
	CountUnbound(ctx context.Context, tenantID int64) (int64, error)
	// AggregateStatsByIDs 根据资源ID列表聚合统计（高性能）
	AggregateStatsByIDs(ctx context.Context, ids []int64, tenantID int64) (*dao.AssetStatsResult, error)
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

func (r *instanceRepository) Update(ctx context.Context, tenantID int64, instance domain.Instance) error {
	return r.dao.Update(ctx, tenantID, r.toDAO(instance))
}

func (r *instanceRepository) GetByID(ctx context.Context, tenantID int64, id int64) (domain.Instance, error) {
	daoInstance, err := r.dao.GetByID(ctx, tenantID, id)
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

func (r *instanceRepository) ListByIDs(ctx context.Context, ids []int64, tenantID int64) ([]domain.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	daoInstances, err := r.dao.ListByIDs(ctx, ids, tenantID)
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

// CountByModelUID 按 model_uid 分组统计租户下实例数量。
func (r *instanceRepository) CountByModelUID(ctx context.Context, tenantID int64) (map[string]int64, error) {
	return r.dao.CountByModelUID(ctx, tenantID)
}

// CountMissingAttribute 统计某模型下缺少指定属性的实例数。
func (r *instanceRepository) CountMissingAttribute(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	return r.dao.CountMissingAttribute(ctx, tenantID, modelUID, fieldUID)
}

func (r *instanceRepository) Delete(ctx context.Context, tenantID int64, id int64) error {
	return r.dao.Delete(ctx, tenantID, id)
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

func (r *instanceRepository) AggregateStatsByIDs(ctx context.Context, ids []int64, tenantID int64) (*dao.AssetStatsResult, error) {
	return r.dao.AggregateStatsByIDs(ctx, ids, tenantID)
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
		AssetID:    filter.AssetID,
		Attributes: filter.Attributes,
		Offset:     filter.Offset,
		Limit:      filter.Limit,
	}
}
