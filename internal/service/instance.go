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
	// attrRepo 用于按模型字段定义校验实例属性（必填/枚举/类型）。
	// 允许为 nil：单元测试与部分历史调用只关心租户隔离，不涉及字段校验。
	attrRepo repository.AttributeRepository
	logger   *elog.Component
}

// NewInstanceService 创建实例服务
func NewInstanceService(repo repository.InstanceRepository, modelRepo repository.ModelRepository, attrRepo repository.AttributeRepository) InstanceService {
	return &instanceService{
		repo:      repo,
		modelRepo: modelRepo,
		attrRepo:  attrRepo,
		logger:    elog.DefaultLogger,
	}
}

// validateModelAttributes 按模型字段定义校验属性。
//
// blockRequired 语义见 domain.ValidateAttributes：
//   - 新建：true（必填缺失直接拒绝）
//   - 更新：false（必填缺失只记警告，否则 4132 条历史缺失必填的实例将无法再编辑）
//
// attrRepo 为 nil 或字段定义拉取失败时**放行**：校验属增强能力，
// 不应因附属查询失败而阻断主流程（与现有「字段定义拉取失败不阻塞表单」的降级策略一致）。
func (s *instanceService) validateModelAttributes(ctx context.Context, instance domain.Instance, blockRequired bool) error {
	if s.attrRepo == nil {
		return nil
	}
	defs, err := s.attrRepo.List(ctx, domain.AttributeFilter{ModelUID: instance.ModelUID, Limit: 500})
	if err != nil {
		s.logger.Warn("拉取模型字段定义失败，跳过字段校验",
			elog.String("model_uid", instance.ModelUID), elog.FieldErr(err))
		return nil
	}
	res := domain.ValidateAttributes(instance.Attributes, defs, blockRequired)
	if len(res.Warnings) > 0 {
		s.logger.Warn("实例字段存在非阻断问题",
			elog.String("model_uid", instance.ModelUID),
			elog.String("asset_id", instance.AssetID),
			elog.Int("warning_count", len(res.Warnings)))
	}
	if res.HasErrors() {
		return fmt.Errorf("%w: %s", errs.ErrInvalidAttributes, res.Error())
	}
	return nil
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

	// 新建：按模型字段定义校验，必填缺失直接拒绝
	if err := s.validateModelAttributes(ctx, instance, true); err != nil {
		return 0, err
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

	// 更新：只做**非阻断**校验（blockRequired=false）。
	// 存量 4132 条实例已缺失必填字段（如 ecs 的 instance_id/instance_name 在真实数据中
	// 从不存在，数据用 asset_id/asset_name）。若更新也硬拦，这些历史实例将无法再编辑，
	// 属明显回归。产品已确认「只对新写入生效」，故此处仅记录警告。
	if err := s.validateModelAttributes(ctx, instance, false); err != nil {
		return err
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
