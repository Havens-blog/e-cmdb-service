package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// ModelGroupService 模型分组服务接口
type ModelGroupService interface {
	Create(ctx context.Context, group domain.ModelGroup) (int64, error)
	Update(ctx context.Context, group domain.ModelGroup) error
	Delete(ctx context.Context, uid string) error
	GetByUID(ctx context.Context, uid string) (domain.ModelGroup, error)
	GetByID(ctx context.Context, id int64) (domain.ModelGroup, error)
	List(ctx context.Context, filter domain.ModelGroupFilter) ([]domain.ModelGroup, int64, error)
	ListWithModels(ctx context.Context) ([]domain.ModelGroupWithModels, error)
	InitBuiltinGroups(ctx context.Context) error
}

type modelGroupService struct {
	groupRepo repository.ModelGroupRepository
	modelRepo repository.ModelRepository
}

// NewModelGroupService 创建模型分组服务
func NewModelGroupService(
	groupRepo repository.ModelGroupRepository,
	modelRepo repository.ModelRepository,
) ModelGroupService {
	return &modelGroupService{
		groupRepo: groupRepo,
		modelRepo: modelRepo,
	}
}

func (s *modelGroupService) Create(ctx context.Context, group domain.ModelGroup) (int64, error) {
	// 检查UID是否已存在
	existing, err := s.groupRepo.GetByUID(ctx, group.UID)
	if err != nil {
		return 0, fmt.Errorf("failed to check existing group: %w", err)
	}
	if existing.ID != 0 {
		return 0, errs.ErrModelGroupExists
	}

	return s.groupRepo.Create(ctx, group)
}

func (s *modelGroupService) Update(ctx context.Context, group domain.ModelGroup) error {
	existing, err := s.groupRepo.GetByUID(ctx, group.UID)
	if err != nil {
		return fmt.Errorf("failed to get group: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrModelGroupNotFound
	}

	// 内置分组只能修改名称和图标
	if existing.IsBuiltin {
		group.IsBuiltin = true
		group.SortOrder = existing.SortOrder
	}

	return s.groupRepo.Update(ctx, group)
}

func (s *modelGroupService) Delete(ctx context.Context, uid string) error {
	existing, err := s.groupRepo.GetByUID(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to get group: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrModelGroupNotFound
	}

	// 内置分组不能删除
	if existing.IsBuiltin {
		return errs.ErrCannotDeleteBuiltin
	}

	// 检查分组下是否有模型
	modelFilter := domain.ModelFilter{
		ModelGroupID: existing.ID,
		Limit:        1,
	}
	models, err := s.modelRepo.List(ctx, modelFilter)
	if err != nil {
		return fmt.Errorf("failed to check models: %w", err)
	}
	if len(models) > 0 {
		return errs.ErrGroupHasModels
	}

	return s.groupRepo.Delete(ctx, uid)
}

func (s *modelGroupService) GetByUID(ctx context.Context, uid string) (domain.ModelGroup, error) {
	group, err := s.groupRepo.GetByUID(ctx, uid)
	if err != nil {
		return domain.ModelGroup{}, fmt.Errorf("failed to get group: %w", err)
	}
	if group.ID == 0 {
		return domain.ModelGroup{}, errs.ErrModelGroupNotFound
	}
	return group, nil
}

func (s *modelGroupService) GetByID(ctx context.Context, id int64) (domain.ModelGroup, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return domain.ModelGroup{}, fmt.Errorf("failed to get group: %w", err)
	}
	if group.ID == 0 {
		return domain.ModelGroup{}, errs.ErrModelGroupNotFound
	}
	return group, nil
}

func (s *modelGroupService) List(ctx context.Context, filter domain.ModelGroupFilter) ([]domain.ModelGroup, int64, error) {
	groups, err := s.groupRepo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list groups: %w", err)
	}

	total, err := s.groupRepo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count groups: %w", err)
	}

	return groups, total, nil
}

// ListWithModels 获取分组列表及其下的模型
func (s *modelGroupService) ListWithModels(ctx context.Context) ([]domain.ModelGroupWithModels, error) {
	// 获取所有分组
	groups, err := s.groupRepo.List(ctx, domain.ModelGroupFilter{Limit: 1000})
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	// 获取所有模型
	models, err := s.modelRepo.List(ctx, domain.ModelFilter{Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	// 按分组ID组织模型
	modelsByGroup := make(map[int64][]domain.Model)
	ungroupedModels := make([]domain.Model, 0)
	for _, model := range models {
		if model.ModelGroupID > 0 {
			modelsByGroup[model.ModelGroupID] = append(modelsByGroup[model.ModelGroupID], model)
		} else {
			ungroupedModels = append(ungroupedModels, model)
		}
	}

	// 组装结果
	result := make([]domain.ModelGroupWithModels, 0, len(groups)+1)
	for _, group := range groups {
		groupWithModels := domain.ModelGroupWithModels{
			ModelGroup: group,
			Models:     modelsByGroup[group.ID],
		}
		if groupWithModels.Models == nil {
			groupWithModels.Models = []domain.Model{}
		}
		result = append(result, groupWithModels)
	}

	// 添加未分组的模型
	if len(ungroupedModels) > 0 {
		result = append(result, domain.ModelGroupWithModels{
			ModelGroup: domain.ModelGroup{
				UID:       "ungrouped",
				Name:      "未分组",
				Icon:      "folder",
				SortOrder: 999,
			},
			Models: ungroupedModels,
		})
	}

	return result, nil
}

func (s *modelGroupService) InitBuiltinGroups(ctx context.Context) error {
	return s.groupRepo.InitBuiltinGroups(ctx)
}
