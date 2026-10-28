package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// InstanceService 资产实例服务接口
type InstanceService interface {
	Create(ctx context.Context, instance domain.Instance) (int64, error)
	CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error)
	Update(ctx context.Context, tenantID int64, instance domain.Instance) error
	GetByID(ctx context.Context, tenantID int64, id int64) (domain.Instance, error)
	GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error)
	List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, int64, error)
	ListByIDs(ctx context.Context, ids []int64, tenantID int64) ([]domain.Instance, error)
	Delete(ctx context.Context, tenantID int64, id int64) error
	DeleteByAccountID(ctx context.Context, accountID int64) error
	Upsert(ctx context.Context, instance domain.Instance) error
	UpsertBatch(ctx context.Context, instances []domain.Instance) error
}

type instanceService struct {
	repo      repository.InstanceRepository
	modelRepo repository.ModelRepository
	logger    *elog.Component
}

// NewInstanceService 创建实例服务
func NewInstanceService(repo repository.InstanceRepository, modelRepo repository.ModelRepository) InstanceService {
	return &instanceService{
		repo:      repo,
		modelRepo: modelRepo,
		logger:    elog.DefaultLogger,
	}
}

func (s *instanceService) Create(ctx context.Context, instance domain.Instance) (int64, error) {
	if err := instance.Validate(); err != nil {
		return 0, err
	}

	model, err := s.modelRepo.GetByUID(ctx, instance.ModelUID)
	if err != nil {
		return 0, fmt.Errorf("failed to check model existence: %w", err)
	}
	if model.ID == 0 {
		return 0, errs.ErrModelNotFound
	}

	existing, err := s.repo.GetByAssetID(ctx, instance.TenantID, instance.ModelUID, instance.AssetID)
	if err != nil {
		return 0, fmt.Errorf("failed to check existing instance: %w", err)
	}
	if existing.ID > 0 {
		return 0, errs.ErrInstanceExists
	}

	return s.repo.Create(ctx, instance)
}

func (s *instanceService) CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error) {
	if len(instances) == 0 {
		return 0, nil
	}

	for _, inst := range instances {
		if err := inst.Validate(); err != nil {
			return 0, err
		}
	}

	return s.repo.CreateBatch(ctx, instances)
}

func (s *instanceService) Update(ctx context.Context, tenantID int64, instance domain.Instance) error {
	if instance.ID == 0 {
		return errs.ErrInstanceNotFound
	}

	return s.repo.Update(ctx, tenantID, instance)
}

func (s *instanceService) GetByID(ctx context.Context, tenantID int64, id int64) (domain.Instance, error) {
	instance, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("failed to get instance: %w", err)
	}
	if instance.ID == 0 {
		return domain.Instance{}, errs.ErrInstanceNotFound
	}
	return instance, nil
}

func (s *instanceService) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error) {
	return s.repo.GetByAssetID(ctx, tenantID, modelUID, assetID)
}

func (s *instanceService) ListByIDs(ctx context.Context, ids []int64, tenantID int64) ([]domain.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.repo.ListByIDs(ctx, ids, tenantID)
}

func (s *instanceService) List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, int64, error) {
	instances, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list instances: %w", err)
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count instances: %w", err)
	}

	return instances, total, nil
}

func (s *instanceService) Delete(ctx context.Context, tenantID int64, id int64) error {
	existing, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrInstanceNotFound
	}

	return s.repo.Delete(ctx, tenantID, id)
}

func (s *instanceService) DeleteByAccountID(ctx context.Context, accountID int64) error {
	return s.repo.DeleteByAccountID(ctx, accountID)
}

func (s *instanceService) Upsert(ctx context.Context, instance domain.Instance) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	return s.repo.Upsert(ctx, instance)
}

func (s *instanceService) UpsertBatch(ctx context.Context, instances []domain.Instance) error {
	for _, inst := range instances {
		if err := inst.Validate(); err != nil {
			return err
		}
		if err := s.repo.Upsert(ctx, inst); err != nil {
			s.logger.Error("failed to upsert instance",
				elog.String("asset_id", inst.AssetID),
				elog.String("model_uid", inst.ModelUID),
				elog.FieldErr(err),
			)
			return err
		}
	}
	return nil
}
