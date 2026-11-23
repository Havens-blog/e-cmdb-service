package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
	"go.mongodb.org/mongo-driver/mongo"
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
	groupRepo    repository.ModelGroupRepository
	logger       *elog.Component
}

// NewModelService 创建模型服务。
//
// groupRepo 用于校验 Update 传入的 model_group_id 指向真实分组：Mongo 没有外键，
// 不做这一步时 {model_group_id: 9999} 会被静默写库，模型随即从所有分组列表里消失
// （分组列表按 model_group_id 反查模型）。
func NewModelService(repo repository.ModelRepository, instanceRepo repository.InstanceRepository, groupRepo repository.ModelGroupRepository) ModelService {
	return &modelService{
		repo:         repo,
		instanceRepo: instanceRepo,
		groupRepo:    groupRepo,
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
	//
	// 裁剪走 Model.Normalize 而不是各自写一份：UID/Category 在这里**只裁剪不校验**
	// （与 Validate 的区别仅此一处），字段清单由 Normalize 独占，新增字段不会漏裁。
	model.Normalize()
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

	if err := s.validateModelGroup(ctx, model.ModelGroupID); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, model); err != nil {
		// DAO 用 MatchedCount==0 报告「目标行已消失」：Exists 与 Update 之间被并发删除时
		// 会走到这里。裸 mongo.ErrNoDocuments 会让 handler 回落成 500 + 驱动原文，
		// 与本次建立的 404 契约冲突。
		if errors.Is(err, mongo.ErrNoDocuments) {
			return errs.ErrModelNotFound
		}
		return err
	}
	return nil
}

// validateModelGroup 校验目标分组的合法性。
//
// 语义：0 = 归入「未分组」（允许）；负数 = 非法；正数 = 必须指向存在的分组。
// 早期实现用 req.ModelGroupID > 0 判断「本次是否修改」，改成指针后 0 有了明确语义，
// 于是负数和幽灵 id 都需要显式拒绝——否则模型会带着一个不存在分组悄悄消失。
func (s *modelService) validateModelGroup(ctx context.Context, groupID int64) error {
	if groupID < 0 {
		return errs.ErrModelGroupInvalid
	}
	if groupID == 0 || s.groupRepo == nil {
		return nil
	}
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("failed to load model group: %w", err)
	}
	if group.ID == 0 {
		return errs.ErrModelGroupNotFound
	}
	return nil
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
