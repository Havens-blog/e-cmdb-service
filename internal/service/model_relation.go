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
	repo repository.ModelRelationTypeRepository
	// instRelRepo 用于删除前统计仍在使用该类型的资产关系。
	// 模型关系类型是与租户无关的元数据，而资产关系是按租户存的，
	// 因此统计必须跨租户（InstanceRelationFilter.TenantID=0 即不加租户谓词）。
	instRelRepo repository.InstanceRelationRepository
	logger      *elog.Component
}

// NewModelRelationTypeService 创建模型关系类型服务
//
// instRelRepo 用于删除保护：仍被资产关系引用的类型不允许删除，
// 否则会留下 relation_type_uid 指向已删类型的孤儿关系（实测 2026-11-19：
// 删除在用类型返回 code=0，关系仍在，但前端已拿不到中文类型名）。
func NewModelRelationTypeService(repo repository.ModelRelationTypeRepository, instRelRepo repository.InstanceRelationRepository) ModelRelationTypeService {
	return &modelRelationTypeService{
		repo:        repo,
		instRelRepo: instRelRepo,
		logger:      elog.DefaultLogger,
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

	// 仍被资产关系引用时拒绝删除。
	//
	// 与 modelService.Delete（模型下有实例则 errs.ErrModelHasInstances）同一套思路：
	// 元数据先删、台账后留会造成用户无法解释的孤儿数据——资产关系还在列表里，
	// 但类型已不在类型清单中，页面只能显示英文 uid，拓扑也失去方向语义。
	if s.instRelRepo != nil {
		count, err := s.instRelRepo.Count(ctx, domain.InstanceRelationFilter{RelationTypeUID: uid})
		if err != nil {
			return fmt.Errorf("failed to count instance relations of type %s: %w", uid, err)
		}
		if count > 0 {
			return &RelationTypeInUseError{RelationTypeUID: uid, Count: count}
		}
	}

	return s.repo.Delete(ctx, uid)
}

// RelationTypeInUseError 表示模型关系类型仍被资产关系引用，不能删除。
//
// 携带引用条数，是因为「有 N 条」与「有 1 条」对用户意味着完全不同的清理工作量；
// 只回一句「正在使用」会让用户去猜到底要不要先清数据。
type RelationTypeInUseError struct {
	RelationTypeUID string
	Count           int64
}

func (e *RelationTypeInUseError) Error() string {
	return fmt.Sprintf("该关系类型正在被 %d 条资产关系使用，请先删除这些资产关系后再删除类型", e.Count)
}

// Is 让 errors.Is(err, errs.ErrRelationTypeInUse) 成立，便于上层统一判定。
func (e *RelationTypeInUseError) Is(target error) bool {
	return target == errs.ErrRelationTypeInUse
}

func (s *modelRelationTypeService) FindByModels(ctx context.Context, sourceUID, targetUID string) ([]domain.ModelRelationType, error) {
	return s.repo.FindByModels(ctx, sourceUID, targetUID)
}
