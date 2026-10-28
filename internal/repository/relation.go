package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/mongo"
)

// InstanceRelationRepository 实例关系仓储接口
type InstanceRelationRepository interface {
	Create(ctx context.Context, relation domain.InstanceRelation) (int64, error)
	CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error)
	GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error)
	List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error)
	Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error)
	Delete(ctx context.Context, tenantID int64, id int64) error
	DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error
	Exists(ctx context.Context, tenantID int64, sourceID, targetID int64, relationTypeUID string) (bool, error)
}

type instanceRelationRepository struct {
	dao dao.InstanceRelationDAO
}

// NewInstanceRelationRepository 创建实例关系仓储
func NewInstanceRelationRepository(dao dao.InstanceRelationDAO) InstanceRelationRepository {
	return &instanceRelationRepository{dao: dao}
}

func (r *instanceRelationRepository) Create(ctx context.Context, relation domain.InstanceRelation) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(relation))
}

func (r *instanceRelationRepository) CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error) {
	daoRelations := make([]dao.InstanceRelation, len(relations))
	for i, rel := range relations {
		daoRelations[i] = r.toDAO(rel)
	}
	return r.dao.CreateBatch(ctx, daoRelations)
}

func (r *instanceRelationRepository) GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error) {
	daoRelation, err := r.dao.GetByID(ctx, tenantID, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.InstanceRelation{}, nil
		}
		return domain.InstanceRelation{}, err
	}
	return r.toDomain(daoRelation), nil
}

func (r *instanceRelationRepository) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error) {
	daoRelations, err := r.dao.List(ctx, r.toDAOFilter(filter))
	if err != nil {
		return nil, err
	}

	relations := make([]domain.InstanceRelation, len(daoRelations))
	for i, daoRel := range daoRelations {
		relations[i] = r.toDomain(daoRel)
	}
	return relations, nil
}

func (r *instanceRelationRepository) Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	return r.dao.Count(ctx, r.toDAOFilter(filter))
}

func (r *instanceRelationRepository) Delete(ctx context.Context, tenantID int64, id int64) error {
	return r.dao.Delete(ctx, tenantID, id)
}

func (r *instanceRelationRepository) DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error {
	return r.dao.DeleteByInstanceID(ctx, tenantID, instanceID)
}

func (r *instanceRelationRepository) Exists(ctx context.Context, tenantID int64, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	return r.dao.Exists(ctx, tenantID, sourceID, targetID, relationTypeUID)
}

func (r *instanceRelationRepository) toDAO(relation domain.InstanceRelation) dao.InstanceRelation {
	return dao.InstanceRelation{
		ID:               relation.ID,
		SourceInstanceID: relation.SourceInstanceID,
		TargetInstanceID: relation.TargetInstanceID,
		RelationTypeUID:  relation.RelationTypeUID,
		TenantID:         relation.TenantID,
	}
}

func (r *instanceRelationRepository) toDomain(daoRelation dao.InstanceRelation) domain.InstanceRelation {
	return domain.InstanceRelation{
		ID:               daoRelation.ID,
		SourceInstanceID: daoRelation.SourceInstanceID,
		TargetInstanceID: daoRelation.TargetInstanceID,
		RelationTypeUID:  daoRelation.RelationTypeUID,
		TenantID:         daoRelation.TenantID,
		CreateTime:       time.UnixMilli(daoRelation.Ctime),
	}
}

func (r *instanceRelationRepository) toDAOFilter(filter domain.InstanceRelationFilter) dao.InstanceRelationFilter {
	return dao.InstanceRelationFilter{
		SourceInstanceID: filter.SourceInstanceID,
		TargetInstanceID: filter.TargetInstanceID,
		RelationTypeUID:  filter.RelationTypeUID,
		TenantID:         filter.TenantID,
		Offset:           filter.Offset,
		Limit:            filter.Limit,
	}
}
