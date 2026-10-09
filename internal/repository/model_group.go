package repository

import (
	"context"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/bson"
)

// ModelGroupRepository 模型分组仓储接口
type ModelGroupRepository interface {
	Create(ctx context.Context, group domain.ModelGroup) (int64, error)
	Update(ctx context.Context, group domain.ModelGroup) error
	Delete(ctx context.Context, uid string) error
	GetByUID(ctx context.Context, uid string) (domain.ModelGroup, error)
	GetByID(ctx context.Context, id int64) (domain.ModelGroup, error)
	List(ctx context.Context, filter domain.ModelGroupFilter) ([]domain.ModelGroup, error)
	Count(ctx context.Context, filter domain.ModelGroupFilter) (int64, error)
	InitBuiltinGroups(ctx context.Context) error
}

type modelGroupRepository struct {
	dao *dao.ModelGroupDAO
}

// NewModelGroupRepository 创建模型分组仓储
func NewModelGroupRepository(dao *dao.ModelGroupDAO) ModelGroupRepository {
	return &modelGroupRepository{dao: dao}
}

func (r *modelGroupRepository) Create(ctx context.Context, group domain.ModelGroup) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(group))
}

func (r *modelGroupRepository) Update(ctx context.Context, group domain.ModelGroup) error {
	return r.dao.Update(ctx, r.toDAO(group))
}

func (r *modelGroupRepository) Delete(ctx context.Context, uid string) error {
	return r.dao.Delete(ctx, uid)
}

func (r *modelGroupRepository) GetByUID(ctx context.Context, uid string) (domain.ModelGroup, error) {
	daoGroup, err := r.dao.GetByUID(ctx, uid)
	if err != nil {
		return domain.ModelGroup{}, err
	}
	return r.toDomain(daoGroup), nil
}

func (r *modelGroupRepository) GetByID(ctx context.Context, id int64) (domain.ModelGroup, error) {
	daoGroup, err := r.dao.GetByID(ctx, id)
	if err != nil {
		return domain.ModelGroup{}, err
	}
	return r.toDomain(daoGroup), nil
}

func (r *modelGroupRepository) List(ctx context.Context, filter domain.ModelGroupFilter) ([]domain.ModelGroup, error) {
	bsonFilter := r.buildFilter(filter)
	daoGroups, err := r.dao.List(ctx, bsonFilter, filter.Offset, filter.Limit)
	if err != nil {
		return nil, err
	}

	groups := make([]domain.ModelGroup, len(daoGroups))
	for i, daoGroup := range daoGroups {
		groups[i] = r.toDomain(daoGroup)
	}
	return groups, nil
}

func (r *modelGroupRepository) Count(ctx context.Context, filter domain.ModelGroupFilter) (int64, error) {
	bsonFilter := r.buildFilter(filter)
	return r.dao.Count(ctx, bsonFilter)
}

func (r *modelGroupRepository) InitBuiltinGroups(ctx context.Context) error {
	builtinGroups := domain.GetBuiltinModelGroups()
	daoGroups := make([]dao.ModelGroup, len(builtinGroups))
	for i, g := range builtinGroups {
		daoGroups[i] = r.toDAO(g)
	}
	return r.dao.InitBuiltinGroups(ctx, daoGroups)
}

func (r *modelGroupRepository) buildFilter(filter domain.ModelGroupFilter) bson.M {
	bsonFilter := bson.M{}
	if filter.UID != "" {
		bsonFilter["uid"] = filter.UID
	}
	if filter.IsBuiltin != nil {
		bsonFilter["is_builtin"] = *filter.IsBuiltin
	}
	return bsonFilter
}

func (r *modelGroupRepository) toDAO(group domain.ModelGroup) dao.ModelGroup {
	return dao.ModelGroup{
		ID:          group.ID,
		UID:         group.UID,
		Name:        group.Name,
		Icon:        group.Icon,
		SortOrder:   group.SortOrder,
		IsBuiltin:   group.IsBuiltin,
		Description: group.Description,
		CreateTime:  group.CreateTime,
		UpdateTime:  group.UpdateTime,
	}
}

func (r *modelGroupRepository) toDomain(daoGroup dao.ModelGroup) domain.ModelGroup {
	return domain.ModelGroup{
		ID:          daoGroup.ID,
		UID:         daoGroup.UID,
		Name:        daoGroup.Name,
		Icon:        daoGroup.Icon,
		SortOrder:   daoGroup.SortOrder,
		IsBuiltin:   daoGroup.IsBuiltin,
		Description: daoGroup.Description,
		CreateTime:  daoGroup.CreateTime,
		UpdateTime:  daoGroup.UpdateTime,
	}
}
