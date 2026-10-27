package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// RelationService 实例关系服务接口
type RelationService interface {
	Create(ctx context.Context, relation domain.InstanceRelation) (int64, error)
	CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error)
	GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error)
	List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, int64, error)
	Delete(ctx context.Context, tenantID int64, id int64) error
	DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error
}

type relationService struct {
	repo   repository.InstanceRelationRepository
	logger *elog.Component
}

// NewRelationService 创建关系服务
func NewRelationService(repo repository.InstanceRelationRepository) RelationService {
	return &relationService{
		repo:   repo,
		logger: elog.DefaultLogger,
	}
}

func (s *relationService) Create(ctx context.Context, relation domain.InstanceRelation) (int64, error) {
	exists, err := s.repo.Exists(ctx, relation.SourceInstanceID, relation.TargetInstanceID, relation.RelationTypeUID)
	if err != nil {
		return 0, fmt.Errorf("failed to check relation existence: %w", err)
	}
	if exists {
		return 0, errs.ErrRelationExists
	}

	return s.repo.Create(ctx, relation)
}

func (s *relationService) CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error) {
	if len(relations) == 0 {
		return 0, nil
	}
	return s.repo.CreateBatch(ctx, relations)
}

func (s *relationService) GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error) {
	relation, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return domain.InstanceRelation{}, fmt.Errorf("failed to get relation: %w", err)
	}
	if relation.ID == 0 {
		return domain.InstanceRelation{}, errs.ErrRelationNotFound
	}
	return relation, nil
}

func (s *relationService) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, int64, error) {
	relations, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list relations: %w", err)
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count relations: %w", err)
	}

	return relations, total, nil
}

func (s *relationService) Delete(ctx context.Context, tenantID int64, id int64) error {
	existing, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get relation: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrRelationNotFound
	}

	return s.repo.Delete(ctx, tenantID, id)
}

func (s *relationService) DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error {
	return s.repo.DeleteByInstanceID(ctx, tenantID, instanceID)
}
