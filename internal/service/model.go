package service

import (
	"context"
	"fmt"
	"strings"

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
	repo         repository.ModelRepository
	instanceRepo repository.InstanceRepository
	logger       *elog.Component
}

// NewModelService 创建模型服务
func NewModelService(repo repository.ModelRepository, instanceRepo repository.InstanceRepository) ModelService {
	return &modelService{
		repo:         repo,
		instanceRepo: instanceRepo,
		logger:       elog.DefaultLogger,
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
	// 只裁剪与校验 Update 实际可变更的字段（评审 #2）：
	// UpdateModelReq 不含 category，且 ecam_model 数据自 e-cam-service 迁移而来，
	// 历史行可能存在空 category；若这里沿用 Model.Validate（要求 Category 非空），
	// 这些模型会再也改不了任何字段，而接口又没有补写 category 的手段。
	// 名称仍必须非空，否则 PUT 可以用单个空格把名称改成「空」。
	model.UID = strings.TrimSpace(model.UID)
	model.Name = strings.TrimSpace(model.Name)
	model.Icon = strings.TrimSpace(model.Icon)
	model.Description = strings.TrimSpace(model.Description)
	model.Category = strings.TrimSpace(model.Category)
	if model.Name == "" {
		return errs.ErrInvalidModelName
	}

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

	count, err := s.instanceRepo.Count(ctx, domain.InstanceFilter{ModelUID: uid})
	if err != nil {
		return fmt.Errorf("failed to count model instances: %w", err)
	}
	if count > 0 {
		return errs.ErrModelHasInstances
	}

	return s.repo.Delete(ctx, uid)
}
