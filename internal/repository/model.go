package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/mongo"
)

// ModelRepository 模型仓储接口
type ModelRepository interface {
	Create(ctx context.Context, model domain.Model) (int64, error)
	GetByUID(ctx context.Context, uid string) (domain.Model, error)
	GetByID(ctx context.Context, id int64) (domain.Model, error)
	List(ctx context.Context, filter domain.ModelFilter) ([]domain.Model, error)
	Count(ctx context.Context, filter domain.ModelFilter) (int64, error)
	Update(ctx context.Context, model domain.Model) error
	Delete(ctx context.Context, uid string) error
	Exists(ctx context.Context, uid string) (bool, error)
}

type modelRepository struct {
	dao dao.ModelDAO
}

// NewModelRepository 创建模型仓储
func NewModelRepository(dao dao.ModelDAO) ModelRepository {
	return &modelRepository{dao: dao}
}

func (r *modelRepository) Create(ctx context.Context, model domain.Model) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(model))
}

func (r *modelRepository) GetByUID(ctx context.Context, uid string) (domain.Model, error) {
	m, err := r.dao.GetByUID(ctx, uid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.Model{}, nil
		}
		return domain.Model{}, err
	}
	return r.toDomain(m), nil
}

func (r *modelRepository) GetByID(ctx context.Context, id int64) (domain.Model, error) {
	m, err := r.dao.GetByID(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.Model{}, nil
		}
		return domain.Model{}, err
	}
	return r.toDomain(m), nil
}

func (r *modelRepository) List(ctx context.Context, filter domain.ModelFilter) ([]domain.Model, error) {
	daoModels, err := r.dao.List(ctx, r.toDAOFilter(filter))
	if err != nil {
		return nil, err
	}

	models := make([]domain.Model, len(daoModels))
	for i, m := range daoModels {
		models[i] = r.toDomain(m)
	}
	return models, nil
}

func (r *modelRepository) Count(ctx context.Context, filter domain.ModelFilter) (int64, error) {
	return r.dao.Count(ctx, r.toDAOFilter(filter))
}

func (r *modelRepository) Update(ctx context.Context, model domain.Model) error {
	return r.dao.Update(ctx, r.toDAO(model))
}

func (r *modelRepository) Delete(ctx context.Context, uid string) error {
	return r.dao.Delete(ctx, uid)
}

func (r *modelRepository) Exists(ctx context.Context, uid string) (bool, error) {
	return r.dao.Exists(ctx, uid)
}

func (r *modelRepository) toDAO(model domain.Model) dao.Model {
	return dao.Model{
		ID:           model.ID,
		UID:          model.UID,
		Name:         model.Name,
		ModelGroupID: model.ModelGroupID,
		ParentUID:    model.ParentUID,
		Category:     model.Category,
		Level:        model.Level,
		Icon:         model.Icon,
		Description:  model.Description,
		Provider:     model.Provider,
		Extensible:   model.Extensible,
	}
}

func (r *modelRepository) toDomain(m dao.Model) domain.Model {
	return domain.Model{
		ID:           m.ID,
		UID:          m.UID,
		Name:         m.Name,
		ModelGroupID: m.ModelGroupID,
		ParentUID:    m.ParentUID,
		Category:     m.Category,
		Level:        m.Level,
		Icon:         m.Icon,
		Description:  m.Description,
		Provider:     m.Provider,
		Extensible:   m.Extensible,
		CreateTime:   time.UnixMilli(m.Ctime),
		UpdateTime:   time.UnixMilli(m.Utime),
	}
}

func (r *modelRepository) toDAOFilter(filter domain.ModelFilter) dao.ModelFilter {
	return dao.ModelFilter{
		Provider:     filter.Provider,
		Category:     filter.Category,
		ParentUID:    filter.ParentUID,
		Level:        filter.Level,
		ModelGroupID: filter.ModelGroupID,
		Extensible:   filter.Extensible,
		Offset:       filter.Offset,
		Limit:        filter.Limit,
	}
}
