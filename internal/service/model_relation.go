package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// ModelRelationTypeService 模型关系类型服务接口
type ModelRelationTypeService interface {
	Create(ctx context.Context, rel domain.ModelRelationType) (int64, error)
	GetByUID(ctx context.Context, uid string) (domain.ModelRelationType, error)
	List(ctx context.Context, filter domain.ModelRelationTypeFilter) ([]domain.ModelRelationType, int64, error)
	Update(ctx context.Context, rel domain.ModelRelationType) error
	Delete(ctx context.Context, uid string) error
	FindByModels(ctx context.Context, sourceUID, targetUID string) ([]domain.ModelRelationType, error)
}

type modelRelationTypeService struct {
	repo   repository.ModelRelationTypeRepository
	logger *elog.Component
}

// NewModelRelationTypeService 创建模型关系类型服务
func NewModelRelationTypeService(repo repository.ModelRelationTypeRepository) ModelRelationTypeService {
	return &modelRelationTypeService{
		repo:   repo,
		logger: elog.DefaultLogger,
	}
}

func (s *modelRelationTypeService) Create(ctx context.Context, rel domain.ModelRelationType) (int64, error) {
	if err := rel.Validate(); err != nil {
		return 0, err
	}

	exists, err := s.repo.Exists(ctx, rel.UID)
	if err != nil {
		return 0, fmt.Errorf("failed to check relation type existence: %w", err)
	}
	if exists {
		return 0, errs.ErrRelationExists
	}

	return s.repo.Create(ctx, rel)
}

func (s *modelRelationTypeService) GetByUID(ctx context.Context, uid string) (domain.ModelRelationType, error) {
	rel, err := s.repo.GetByUID(ctx, uid)
	if err != nil {
		return domain.ModelRelationType{}, fmt.Errorf("failed to get relation type: %w", err)
	}
	if rel.ID == 0 {
		return domain.ModelRelationType{}, errs.ErrRelationNotFound
	}
	return rel, nil
}

func (s *modelRelationTypeService) List(ctx context.Context, filter domain.ModelRelationTypeFilter) ([]domain.ModelRelationType, int64, error) {
	rels, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list relation types: %w", err)
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count relation types: %w", err)
	}

	return rels, total, nil
}

func (s *modelRelationTypeService) Update(ctx context.Context, rel domain.ModelRelationType) error {
	exists, err := s.repo.Exists(ctx, rel.UID)
	if err != nil {
		return fmt.Errorf("failed to check relation type existence: %w", err)
	}
	if !exists {
		return errs.ErrRelationNotFound
	}

	return s.repo.Update(ctx, rel)
}

func (s *modelRelationTypeService) Delete(ctx context.Context, uid string) error {
	exists, err := s.repo.Exists(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to check relation type existence: %w", err)
	}
	if !exists {
		return errs.ErrRelationNotFound
	}

	return s.repo.Delete(ctx, uid)
}

func (s *modelRelationTypeService) FindByModels(ctx context.Context, sourceUID, targetUID string) ([]domain.ModelRelationType, error) {
	return s.repo.FindByModels(ctx, sourceUID, targetUID)
}
