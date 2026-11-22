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
	repo      repository.ModelRelationTypeRepository
	modelRepo repository.ModelRepository
	logger    *elog.Component
}

// NewModelRelationTypeService 创建模型关系类型服务。
//
// modelRepo 只用于 Create 时校验 source/target 模型确实存在；传 nil 时跳过该校验
// （保留给尚未装配模型仓储的测试与旧调用点，生产装配见 internal/module.go）。
func NewModelRelationTypeService(repo repository.ModelRelationTypeRepository, modelRepo repository.ModelRepository) ModelRelationTypeService {
	return &modelRelationTypeService{
		repo:      repo,
		modelRepo: modelRepo,
		logger:    elog.DefaultLogger,
	}
}

func (s *modelRelationTypeService) Create(ctx context.Context, rel domain.ModelRelationType) (int64, error) {
	// 先裁剪再校验：`binding:"required"` 只拒空串，单个空格能进到这里，
	// 存成 " " 的 uid 会让关系类型在列表里「看得见、选不着」。
	rel.UID = strings.TrimSpace(rel.UID)
	rel.Name = strings.TrimSpace(rel.Name)
	rel.SourceModelUID = strings.TrimSpace(rel.SourceModelUID)
	rel.TargetModelUID = strings.TrimSpace(rel.TargetModelUID)

	if err := rel.Validate(); err != nil {
		return 0, err
	}

	if err := s.ensureModelsExist(ctx, rel.SourceModelUID, rel.TargetModelUID); err != nil {
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

// ensureModelsExist 校验关系类型声明的源/目标模型都存在。
//
// 关系类型是「哪些模型之间可以建立关系」的声明，声明本身指向不存在的模型时，
// 实例关系页会按该 uid 检索资产，候选恒为 0，用户只能看到「没有匹配的资产」而不知原因。
func (s *modelRelationTypeService) ensureModelsExist(ctx context.Context, sourceModelUID, targetModelUID string) error {
	if s.modelRepo == nil {
		return nil
	}

	source, err := s.modelRepo.GetByUID(ctx, sourceModelUID)
	if err != nil {
		return fmt.Errorf("failed to load source model %s: %w", sourceModelUID, err)
	}
	if source.ID == 0 {
		return fmt.Errorf("%w: model_uid=%s", errs.ErrRelationSourceModelNotFound, sourceModelUID)
	}

	target, err := s.modelRepo.GetByUID(ctx, targetModelUID)
	if err != nil {
		return fmt.Errorf("failed to load target model %s: %w", targetModelUID, err)
	}
	if target.ID == 0 {
		return fmt.Errorf("%w: model_uid=%s", errs.ErrRelationTargetModelNotFound, targetModelUID)
	}

	return nil
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
	// 与 Create 保持同一套裁剪 + 校验：HTTP 层 binding:"required" 放行 " "，
	// 若这里不裁剪不判空，PUT 就能把关系类型改成空白名称（列表与拓扑边标签都为空白）。
	rel.UID = strings.TrimSpace(rel.UID)
	rel.Name = strings.TrimSpace(rel.Name)
	rel.SourceModelUID = strings.TrimSpace(rel.SourceModelUID)
	rel.TargetModelUID = strings.TrimSpace(rel.TargetModelUID)
	if err := rel.Validate(); err != nil {
		return err
	}

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
