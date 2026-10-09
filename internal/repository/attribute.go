package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/mongo"
)

// AttributeRepository 属性仓储接口
type AttributeRepository interface {
	Create(ctx context.Context, attr domain.Attribute) (int64, error)
	GetByID(ctx context.Context, id int64) (domain.Attribute, error)
	GetByFieldUID(ctx context.Context, modelUID, fieldUID string) (domain.Attribute, error)
	List(ctx context.Context, filter domain.AttributeFilter) ([]domain.Attribute, error)
	Count(ctx context.Context, filter domain.AttributeFilter) (int64, error)
	Update(ctx context.Context, attr domain.Attribute) error
	Delete(ctx context.Context, id int64) error
	DeleteByModelUID(ctx context.Context, modelUID string) error
	Exists(ctx context.Context, modelUID, fieldUID string) (bool, error)
}

type attributeRepository struct {
	dao dao.AttributeDAO
}

// NewAttributeRepository 创建属性仓储
func NewAttributeRepository(dao dao.AttributeDAO) AttributeRepository {
	return &attributeRepository{dao: dao}
}

func (r *attributeRepository) Create(ctx context.Context, attr domain.Attribute) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(attr))
}

func (r *attributeRepository) GetByID(ctx context.Context, id int64) (domain.Attribute, error) {
	daoAttr, err := r.dao.GetByID(ctx, id)
	if err != nil {
		return domain.Attribute{}, err
	}
	return r.toDomain(daoAttr), nil
}

func (r *attributeRepository) GetByFieldUID(ctx context.Context, modelUID, fieldUID string) (domain.Attribute, error) {
	daoAttr, err := r.dao.GetByFieldUID(ctx, modelUID, fieldUID)
	if err != nil {
		return domain.Attribute{}, err
	}
	return r.toDomain(daoAttr), nil
}

func (r *attributeRepository) List(ctx context.Context, filter domain.AttributeFilter) ([]domain.Attribute, error) {
	daoFilter := dao.AttributeFilter{
		ModelUID:   filter.ModelUID,
		GroupID:    filter.GroupID,
		FieldType:  filter.FieldType,
		Display:    filter.Display,
		Required:   filter.Required,
		Searchable: filter.Searchable,
		Offset:     filter.Offset,
		Limit:      filter.Limit,
	}
	daoAttrs, err := r.dao.List(ctx, daoFilter)
	if err != nil {
		return nil, err
	}
	attrs := make([]domain.Attribute, len(daoAttrs))
	for i, daoAttr := range daoAttrs {
		attrs[i] = r.toDomain(daoAttr)
	}
	return attrs, nil
}

func (r *attributeRepository) Count(ctx context.Context, filter domain.AttributeFilter) (int64, error) {
	daoFilter := dao.AttributeFilter{
		ModelUID:   filter.ModelUID,
		GroupID:    filter.GroupID,
		FieldType:  filter.FieldType,
		Display:    filter.Display,
		Required:   filter.Required,
		Searchable: filter.Searchable,
	}
	return r.dao.Count(ctx, daoFilter)
}

func (r *attributeRepository) Update(ctx context.Context, attr domain.Attribute) error {
	return r.dao.Update(ctx, r.toDAO(attr))
}

func (r *attributeRepository) Delete(ctx context.Context, id int64) error {
	return r.dao.Delete(ctx, id)
}

func (r *attributeRepository) DeleteByModelUID(ctx context.Context, modelUID string) error {
	return r.dao.DeleteByModelUID(ctx, modelUID)
}

func (r *attributeRepository) Exists(ctx context.Context, modelUID, fieldUID string) (bool, error) {
	return r.dao.Exists(ctx, modelUID, fieldUID)
}

func (r *attributeRepository) toDAO(attr domain.Attribute) dao.Attribute {
	return dao.Attribute{
		ID:          attr.ID,
		FieldUID:    attr.FieldUID,
		FieldName:   attr.FieldName,
		FieldType:   attr.FieldType,
		ModelUID:    attr.ModelUID,
		GroupID:     attr.GroupID,
		DisplayName: attr.DisplayName,
		Display:     attr.Display,
		Index:       attr.Index,
		Required:    attr.Required,
		Editable:    attr.Editable,
		Searchable:  attr.Searchable,
		Unique:      attr.Unique,
		Secure:      attr.Secure,
		Link:        attr.Link,
		LinkModel:   attr.LinkModel,
		Option:      attr.Option,
		Default:     attr.Default,
		Placeholder: attr.Placeholder,
		Description: attr.Description,
	}
}

func (r *attributeRepository) toDomain(daoAttr dao.Attribute) domain.Attribute {
	return domain.Attribute{
		ID:          daoAttr.ID,
		FieldUID:    daoAttr.FieldUID,
		FieldName:   daoAttr.FieldName,
		FieldType:   daoAttr.FieldType,
		ModelUID:    daoAttr.ModelUID,
		GroupID:     daoAttr.GroupID,
		DisplayName: daoAttr.DisplayName,
		Display:     daoAttr.Display,
		Index:       daoAttr.Index,
		Required:    daoAttr.Required,
		Editable:    daoAttr.Editable,
		Searchable:  daoAttr.Searchable,
		Unique:      daoAttr.Unique,
		Secure:      daoAttr.Secure,
		Link:        daoAttr.Link,
		LinkModel:   daoAttr.LinkModel,
		Option:      daoAttr.Option,
		Default:     daoAttr.Default,
		Placeholder: daoAttr.Placeholder,
		Description: daoAttr.Description,
		CreateTime:  time.UnixMilli(daoAttr.Ctime),
		UpdateTime:  time.UnixMilli(daoAttr.Utime),
	}
}

// AttributeGroupRepository 属性分组仓储接口
type AttributeGroupRepository interface {
	Create(ctx context.Context, group domain.AttributeGroup) (int64, error)
	GetByID(ctx context.Context, id int64) (domain.AttributeGroup, error)
	GetByUID(ctx context.Context, modelUID, uid string) (domain.AttributeGroup, error)
	List(ctx context.Context, modelUID string) ([]domain.AttributeGroup, error)
	Update(ctx context.Context, group domain.AttributeGroup) error
	Delete(ctx context.Context, id int64) error
	DeleteByModelUID(ctx context.Context, modelUID string) error
	Upsert(ctx context.Context, group domain.AttributeGroup) error
}

type attributeGroupRepository struct {
	dao dao.AttributeGroupDAO
}

// NewAttributeGroupRepository 创建属性分组仓储
func NewAttributeGroupRepository(dao dao.AttributeGroupDAO) AttributeGroupRepository {
	return &attributeGroupRepository{dao: dao}
}

func (r *attributeGroupRepository) Create(ctx context.Context, group domain.AttributeGroup) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(group))
}

func (r *attributeGroupRepository) GetByID(ctx context.Context, id int64) (domain.AttributeGroup, error) {
	daoGroup, err := r.dao.GetByID(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.AttributeGroup{}, nil
		}
		return domain.AttributeGroup{}, err
	}
	return r.toDomain(daoGroup), nil
}

func (r *attributeGroupRepository) GetByUID(ctx context.Context, modelUID, uid string) (domain.AttributeGroup, error) {
	daoGroup, err := r.dao.GetByUID(ctx, modelUID, uid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.AttributeGroup{}, nil
		}
		return domain.AttributeGroup{}, err
	}
	return r.toDomain(daoGroup), nil
}

func (r *attributeGroupRepository) List(ctx context.Context, modelUID string) ([]domain.AttributeGroup, error) {
	daoGroups, err := r.dao.List(ctx, modelUID)
	if err != nil {
		return nil, err
	}
	groups := make([]domain.AttributeGroup, len(daoGroups))
	for i, daoGroup := range daoGroups {
		groups[i] = r.toDomain(daoGroup)
	}
	return groups, nil
}

func (r *attributeGroupRepository) Update(ctx context.Context, group domain.AttributeGroup) error {
	return r.dao.Update(ctx, r.toDAO(group))
}

func (r *attributeGroupRepository) Delete(ctx context.Context, id int64) error {
	return r.dao.Delete(ctx, id)
}

func (r *attributeGroupRepository) DeleteByModelUID(ctx context.Context, modelUID string) error {
	return r.dao.DeleteByModelUID(ctx, modelUID)
}

func (r *attributeGroupRepository) Upsert(ctx context.Context, group domain.AttributeGroup) error {
	return r.dao.Upsert(ctx, r.toDAO(group))
}

func (r *attributeGroupRepository) toDAO(group domain.AttributeGroup) dao.AttributeGroup {
	return dao.AttributeGroup{
		ID:          group.ID,
		UID:         group.UID,
		Name:        group.Name,
		ModelUID:    group.ModelUID,
		Index:       group.Index,
		IsBuiltin:   group.IsBuiltin,
		Description: group.Description,
	}
}

func (r *attributeGroupRepository) toDomain(daoGroup dao.AttributeGroup) domain.AttributeGroup {
	return domain.AttributeGroup{
		ID:          daoGroup.ID,
		UID:         daoGroup.UID,
		Name:        daoGroup.Name,
		ModelUID:    daoGroup.ModelUID,
		Index:       daoGroup.Index,
		IsBuiltin:   daoGroup.IsBuiltin,
		Description: daoGroup.Description,
		CreateTime:  time.UnixMilli(daoGroup.Ctime),
		UpdateTime:  time.UnixMilli(daoGroup.Utime),
	}
}
