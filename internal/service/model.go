package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// ModelService 模型服务接口
type ModelService interface {
	Create(ctx context.Context, model domain.Model) (int64, error)
	GetByUID(ctx context.Context, uid string) (domain.Model, error)
	GetByID(ctx context.Context, id int64) (domain.Model, error)
	List(ctx context.Context, filter domain.ModelFilter) ([]domain.Model, int64, error)
	Update(ctx context.Context, model domain.Model) error
	Delete(ctx context.Context, uid string) error
}

type modelService struct {
	repo   repository.ModelRepository
	logger *elog.Component
}

// NewModelService 创建模型服务
func NewModelService(repo repository.ModelRepository) ModelService {
	return &modelService{
		repo:   repo,
		logger: elog.DefaultLogger,
	}
}

func (s *modelService) Create(ctx context.Context, model domain.Model) (int64, error) {
	if err := model.Validate(); err != nil {
		return 0, err
	}

	exists, err := s.repo.Exists(ctx, model.UID)
	if err != nil {
		return 0, fmt.Errorf("failed to check model existence: %w", err)
	}
	if exists {
		return 0, errs.ErrModelExists
	}

	return s.repo.Create(ctx, model)
}

func (s *modelService) GetByUID(ctx context.Context, uid string) (domain.Model, error) {
	model, err := s.repo.GetByUID(ctx, uid)
	if err != nil {
		return domain.Model{}, fmt.Errorf("failed to get model: %w", err)
	}
	if model.ID == 0 {
		return domain.Model{}, errs.ErrModelNotFound
	}
	return model, nil
}

func (s *modelService) GetByID(ctx context.Context, id int64) (domain.Model, error) {
	model, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Model{}, fmt.Errorf("failed to get model: %w", err)
	}
	if model.ID == 0 {
		return domain.Model{}, errs.ErrModelNotFound
	}
	return model, nil
}

func (s *modelService) List(ctx context.Context, filter domain.ModelFilter) ([]domain.Model, int64, error) {
	models, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list models: %w", err)
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count models: %w", err)
	}

	return models, total, nil
}

func (s *modelService) Update(ctx context.Context, model domain.Model) error {
	exists, err := s.repo.Exists(ctx, model.UID)
	if err != nil {
		return fmt.Errorf("failed to check model existence: %w", err)
	}
	if !exists {
		return errs.ErrModelNotFound
	}

	return s.repo.Update(ctx, model)
}

func (s *modelService) Delete(ctx context.Context, uid string) error {
	exists, err := s.repo.Exists(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to check model existence: %w", err)
	}
	if !exists {
		return errs.ErrModelNotFound
	}

	return s.repo.Delete(ctx, uid)
}
