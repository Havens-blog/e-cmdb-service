package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/mongo"
)

// ModelRelationTypeRepository 模型关系类型仓储接口
type ModelRelationTypeRepository interface {
	Create(ctx context.Context, rel domain.ModelRelationType) (int64, error)
	GetByUID(ctx context.Context, uid string) (domain.ModelRelationType, error)
	GetByID(ctx context.Context, id int64) (domain.ModelRelationType, error)
	List(ctx context.Context, filter domain.ModelRelationTypeFilter) ([]domain.ModelRelationType, error)
	Count(ctx context.Context, filter domain.ModelRelationTypeFilter) (int64, error)
	Update(ctx context.Context, rel domain.ModelRelationType) error
	Delete(ctx context.Context, uid string) error
	Exists(ctx context.Context, uid string) (bool, error)
	FindByModels(ctx context.Context, sourceUID, targetUID string) ([]domain.ModelRelationType, error)
}

type modelRelationTypeRepository struct {
	dao dao.ModelRelationTypeDAO
}

// NewModelRelationTypeRepository 创建模型关系类型仓储
func NewModelRelationTypeRepository(dao dao.ModelRelationTypeDAO) ModelRelationTypeRepository {
	return &modelRelationTypeRepository{dao: dao}
}

func (r *modelRelationTypeRepository) Create(ctx context.Context, rel domain.ModelRelationType) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(rel))
}

func (r *modelRelationTypeRepository) GetByUID(ctx context.Context, uid string) (domain.ModelRelationType, error) {
	d, err := r.dao.GetByUID(ctx, uid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.ModelRelationType{}, nil
		}
		return domain.ModelRelationType{}, err
	}
	return r.toDomain(d), nil
}

func (r *modelRelationTypeRepository) GetByID(ctx context.Context, id int64) (domain.ModelRelationType, error) {
	d, err := r.dao.GetByID(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.ModelRelationType{}, nil
		}
		return domain.ModelRelationType{}, err
	}
	return r.toDomain(d), nil
}

func (r *modelRelationTypeRepository) List(ctx context.Context, filter domain.ModelRelationTypeFilter) ([]domain.ModelRelationType, error) {
	daoList, err := r.dao.List(ctx, r.toDAOFilter(filter))
	if err != nil {
		return nil, err
	}
	result := make([]domain.ModelRelationType, len(daoList))
	for i, d := range daoList {
		result[i] = r.toDomain(d)
	}
	return result, nil
}

func (r *modelRelationTypeRepository) Count(ctx context.Context, filter domain.ModelRelationTypeFilter) (int64, error) {
	return r.dao.Count(ctx, r.toDAOFilter(filter))
}

func (r *modelRelationTypeRepository) Update(ctx context.Context, rel domain.ModelRelationType) error {
	return r.dao.Update(ctx, r.toDAO(rel))
}

func (r *modelRelationTypeRepository) Delete(ctx context.Context, uid string) error {
	return r.dao.Delete(ctx, uid)
}

func (r *modelRelationTypeRepository) Exists(ctx context.Context, uid string) (bool, error) {
	return r.dao.Exists(ctx, uid)
}

func (r *modelRelationTypeRepository) FindByModels(ctx context.Context, sourceUID, targetUID string) ([]domain.ModelRelationType, error) {
	daoList, err := r.dao.FindByModels(ctx, sourceUID, targetUID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ModelRelationType, len(daoList))
	for i, d := range daoList {
		result[i] = r.toDomain(d)
	}
	return result, nil
}

func (r *modelRelationTypeRepository) toDAO(rel domain.ModelRelationType) dao.ModelRelationType {
	return dao.ModelRelationType{
		ID:             rel.ID,
		UID:            rel.UID,
		Name:           rel.Name,
		SourceModelUID: rel.SourceModelUID,
		TargetModelUID: rel.TargetModelUID,
		RelationType:   rel.RelationType,
		Direction:      rel.Direction,
		SourceToTarget: rel.SourceToTarget,
		TargetToSource: rel.TargetToSource,
		Description:    rel.Description,
	}
}

func (r *modelRelationTypeRepository) toDomain(d dao.ModelRelationType) domain.ModelRelationType {
	return domain.ModelRelationType{
		ID:             d.ID,
		UID:            d.UID,
		Name:           d.Name,
		SourceModelUID: d.SourceModelUID,
		TargetModelUID: d.TargetModelUID,
		RelationType:   d.RelationType,
		Direction:      d.Direction,
		SourceToTarget: d.SourceToTarget,
		TargetToSource: d.TargetToSource,
		Description:    d.Description,
		CreateTime:     time.UnixMilli(d.Ctime),
		UpdateTime:     time.UnixMilli(d.Utime),
	}
}

func (r *modelRelationTypeRepository) toDAOFilter(filter domain.ModelRelationTypeFilter) dao.ModelRelationTypeFilter {
	return dao.ModelRelationTypeFilter{
		SourceModelUID: filter.SourceModelUID,
		TargetModelUID: filter.TargetModelUID,
		RelationType:   filter.RelationType,
		Offset:         filter.Offset,
		Limit:          filter.Limit,
	}
}
