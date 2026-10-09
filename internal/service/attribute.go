package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// AttributeService 属性服务接口
type AttributeService interface {
	// 属性管理
	CreateAttribute(ctx context.Context, attr domain.Attribute) (int64, error)
	GetAttribute(ctx context.Context, id int64) (domain.Attribute, error)
	GetAttributeByFieldUID(ctx context.Context, modelUID, fieldUID string) (domain.Attribute, error)
	ListAttributes(ctx context.Context, filter domain.AttributeFilter) ([]domain.Attribute, int64, error)
	UpdateAttribute(ctx context.Context, attr domain.Attribute) error
	DeleteAttribute(ctx context.Context, id int64) error

	// 属性分组管理
	CreateAttributeGroup(ctx context.Context, group domain.AttributeGroup) (int64, error)
	GetAttributeGroup(ctx context.Context, id int64) (domain.AttributeGroup, error)
	ListAttributeGroups(ctx context.Context, modelUID string) ([]domain.AttributeGroup, error)
	UpdateAttributeGroup(ctx context.Context, group domain.AttributeGroup) error
	DeleteAttributeGroup(ctx context.Context, id int64) error
	InitBuiltinGroups(ctx context.Context, modelUID string) error

	// 带分组的属性列表
	ListAttributesWithGroups(ctx context.Context, modelUID string) ([]domain.AttributeGroupWithAttrs, error)

	// 获取字段类型列表
	GetFieldTypes() []map[string]string
}

type attributeService struct {
	attrRepo      repository.AttributeRepository
	attrGroupRepo repository.AttributeGroupRepository
	modelRepo     repository.ModelRepository
}

// NewAttributeService 创建属性服务
func NewAttributeService(
	attrRepo repository.AttributeRepository,
	attrGroupRepo repository.AttributeGroupRepository,
	modelRepo repository.ModelRepository,
) AttributeService {
	return &attributeService{
		attrRepo:      attrRepo,
		attrGroupRepo: attrGroupRepo,
		modelRepo:     modelRepo,
	}
}

// CreateAttribute 创建属性
func (s *attributeService) CreateAttribute(ctx context.Context, attr domain.Attribute) (int64, error) {
	// 验证属性数据
	if err := attr.Validate(); err != nil {
		return 0, err
	}

	// 检查模型是否存在
	exists, err := s.modelRepo.Exists(ctx, attr.ModelUID)
	if err != nil {
		return 0, fmt.Errorf("failed to check model existence: %w", err)
	}
	if !exists {
		return 0, errs.ErrModelNotFound
	}

	// 检查字段UID是否已存在
	attrExists, err := s.attrRepo.Exists(ctx, attr.ModelUID, attr.FieldUID)
	if err != nil {
		return 0, fmt.Errorf("failed to check attribute existence: %w", err)
	}
	if attrExists {
		return 0, errs.ErrAttributeExists
	}

	// 设置默认值
	if attr.GroupID == 0 {
		// 获取自定义分组
		customGroup, err := s.attrGroupRepo.GetByUID(ctx, attr.ModelUID, domain.ATTR_GROUP_CUSTOM)
		if err == nil && customGroup.ID > 0 {
			attr.GroupID = customGroup.ID
		}
	}

	return s.attrRepo.Create(ctx, attr)
}

// GetAttribute 获取属性
func (s *attributeService) GetAttribute(ctx context.Context, id int64) (domain.Attribute, error) {
	return s.attrRepo.GetByID(ctx, id)
}

// GetAttributeByFieldUID 根据字段UID获取属性
func (s *attributeService) GetAttributeByFieldUID(ctx context.Context, modelUID, fieldUID string) (domain.Attribute, error) {
	return s.attrRepo.GetByFieldUID(ctx, modelUID, fieldUID)
}

// ListAttributes 获取属性列表
func (s *attributeService) ListAttributes(ctx context.Context, filter domain.AttributeFilter) ([]domain.Attribute, int64, error) {
	attrs, err := s.attrRepo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.attrRepo.Count(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return attrs, total, nil
}

// UpdateAttribute 更新属性
func (s *attributeService) UpdateAttribute(ctx context.Context, attr domain.Attribute) error {
	// 检查属性是否存在
	existing, err := s.attrRepo.GetByID(ctx, attr.ID)
	if err != nil {
		return errs.ErrAttributeNotFound
	}

	// 不允许修改字段UID和模型UID
	attr.FieldUID = existing.FieldUID
	attr.ModelUID = existing.ModelUID

	return s.attrRepo.Update(ctx, attr)
}

// DeleteAttribute 删除属性
func (s *attributeService) DeleteAttribute(ctx context.Context, id int64) error {
	return s.attrRepo.Delete(ctx, id)
}

// CreateAttributeGroup 创建属性分组
func (s *attributeService) CreateAttributeGroup(ctx context.Context, group domain.AttributeGroup) (int64, error) {
	if group.UID == "" || group.Name == "" || group.ModelUID == "" {
		return 0, errs.ErrInvalidAttributeGroup
	}

	// 检查模型是否存在
	exists, err := s.modelRepo.Exists(ctx, group.ModelUID)
	if err != nil {
		return 0, fmt.Errorf("failed to check model existence: %w", err)
	}
	if !exists {
		return 0, errs.ErrModelNotFound
	}

	return s.attrGroupRepo.Create(ctx, group)
}

// GetAttributeGroup 获取属性分组
func (s *attributeService) GetAttributeGroup(ctx context.Context, id int64) (domain.AttributeGroup, error) {
	return s.attrGroupRepo.GetByID(ctx, id)
}

// ListAttributeGroups 获取属性分组列表
func (s *attributeService) ListAttributeGroups(ctx context.Context, modelUID string) ([]domain.AttributeGroup, error) {
	return s.attrGroupRepo.List(ctx, modelUID)
}

// UpdateAttributeGroup 更新属性分组
func (s *attributeService) UpdateAttributeGroup(ctx context.Context, group domain.AttributeGroup) error {
	existing, err := s.attrGroupRepo.GetByID(ctx, group.ID)
	if err != nil || existing.ID == 0 {
		return errs.ErrAttributeGroupNotFound
	}

	// 内置分组不允许修改UID
	if existing.IsBuiltin {
		group.UID = existing.UID
		group.IsBuiltin = true
	}

	return s.attrGroupRepo.Update(ctx, group)
}

// DeleteAttributeGroup 删除属性分组
func (s *attributeService) DeleteAttributeGroup(ctx context.Context, id int64) error {
	group, err := s.attrGroupRepo.GetByID(ctx, id)
	if err != nil || group.ID == 0 {
		return errs.ErrAttributeGroupNotFound
	}

	// 内置分组不允许删除
	if group.IsBuiltin {
		return errs.ErrBuiltinGroupCannotDelete
	}

	return s.attrGroupRepo.Delete(ctx, id)
}

// InitBuiltinGroups 初始化内置属性分组
func (s *attributeService) InitBuiltinGroups(ctx context.Context, modelUID string) error {
	builtinGroups := domain.GetBuiltinAttributeGroups(modelUID)
	for _, group := range builtinGroups {
		if err := s.attrGroupRepo.Upsert(ctx, group); err != nil {
			return fmt.Errorf("failed to init builtin group %s: %w", group.UID, err)
		}
	}
	return nil
}

// ListAttributesWithGroups 获取带分组的属性列表
func (s *attributeService) ListAttributesWithGroups(ctx context.Context, modelUID string) ([]domain.AttributeGroupWithAttrs, error) {
	if modelUID == "" {
		return nil, fmt.Errorf("model_uid is required")
	}

	// 检查模型是否存在
	exists, err := s.modelRepo.Exists(ctx, modelUID)
	if err != nil {
		return nil, fmt.Errorf("failed to check model existence: %w", err)
	}
	if !exists {
		// 模型不存在时返回空分组列表，而不是报错
		return []domain.AttributeGroupWithAttrs{}, nil
	}

	// 获取所有分组
	groups, err := s.attrGroupRepo.List(ctx, modelUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list attribute groups: %w", err)
	}

	// 如果没有分组，初始化内置分组
	if len(groups) == 0 {
		if initErr := s.InitBuiltinGroups(ctx, modelUID); initErr != nil {
			// 初始化失败不阻塞请求，返回空分组
			return []domain.AttributeGroupWithAttrs{}, nil
		}
		groups, err = s.attrGroupRepo.List(ctx, modelUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list attribute groups after init: %w", err)
		}
	}

	// 获取所有属性
	attrs, err := s.attrRepo.List(ctx, domain.AttributeFilter{ModelUID: modelUID})
	if err != nil {
		return nil, fmt.Errorf("failed to list attributes: %w", err)
	}

	// 按分组组织属性
	groupMap := make(map[int64][]domain.Attribute)
	for _, attr := range attrs {
		groupMap[attr.GroupID] = append(groupMap[attr.GroupID], attr)
	}

	// 构建结果
	result := make([]domain.AttributeGroupWithAttrs, len(groups))
	for i, group := range groups {
		result[i] = domain.AttributeGroupWithAttrs{
			AttributeGroup: group,
			Attributes:     groupMap[group.ID],
		}
		if result[i].Attributes == nil {
			result[i].Attributes = []domain.Attribute{}
		}
	}

	return result, nil
}

// GetFieldTypes 获取字段类型列表
func (s *attributeService) GetFieldTypes() []map[string]string {
	return domain.GetFieldTypes()
}
