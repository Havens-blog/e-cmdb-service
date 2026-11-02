package service

import (
	"context"
	"fmt"
	"time"

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
	// relationRepo 用于删除实例时**级联解除关系**（已确认规则第 5 条）。
	// 允许为 nil：部分单元测试不涉及关系，此时跳过级联（仅记录警告）。
	relationRepo repository.InstanceRelationRepository
	// syncQualityIssueRepo 用于同步链路「记录但不阻断」：把字段校验问题写入
	// 可查询的问题清单。允许为 nil（校验/记录退化为仅日志）。
	syncQualityIssueRepo repository.SyncQualityIssueRepository
	// changeHistoryRepo 用于实例变更审计（变更前后值写入 ecam_change_history）。
	// 允许为 nil（审计退化为仅日志，绝不阻断业务操作）。
	changeHistoryRepo repository.ChangeHistoryRepository
	logger            *elog.Component
}

// NewInstanceService 创建实例服务
func NewInstanceService(
	repo repository.InstanceRepository,
	modelRepo repository.ModelRepository,
	attrRepo repository.AttributeRepository,
	relationRepo repository.InstanceRelationRepository,
	syncQualityIssueRepo repository.SyncQualityIssueRepository,
	changeHistoryRepo repository.ChangeHistoryRepository,
) InstanceService {
	return &instanceService{
		repo:                 repo,
		modelRepo:            modelRepo,
		attrRepo:             attrRepo,
		relationRepo:         relationRepo,
		syncQualityIssueRepo: syncQualityIssueRepo,
		changeHistoryRepo:    changeHistoryRepo,
		logger:               elog.DefaultLogger,
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

// recordSyncQualityIssues 把一批实例的字段校验问题（warn-only）写入问题清单。
//
// 方案 B（2026-11-02 产品确认）：「记录但不阻断」——同步采集不中断，
// 但把每个不合规实例的必填/枚举/类型问题按自然键去重写入可查询清单。
//
// 效率：所有模型的字段定义只加载一次（groupByModelUID），
// 避免对同步批次里的每条实例都发一次查询。
// 校验与写入失败均只记日志、不返回错误，保证采集永不因质量记录而中断。
func (s *instanceService) recordSyncQualityIssues(ctx context.Context, tenantID int64, instances []domain.Instance) {
	if s.attrRepo == nil {
		return
	}

	defs, err := s.attrRepo.List(ctx, domain.AttributeFilter{Limit: 100000})
	if err != nil {
		s.logger.Warn("同步质量记录：拉取字段定义失败，跳过", elog.FieldErr(err))
		return
	}
	defsByModel := make(map[string][]domain.Attribute, 64)
	for _, d := range defs {
		defsByModel[d.ModelUID] = append(defsByModel[d.ModelUID], d)
	}

	for _, inst := range instances {
		modelDefs, ok := defsByModel[inst.ModelUID]
		if !ok || len(modelDefs) == 0 {
			continue
		}
		res := domain.ValidateAttributes(inst.Attributes, modelDefs, false)
		if len(res.Warnings) == 0 {
			continue
		}
		for _, w := range res.Warnings {
			issue := domain.SyncQualityIssue{
				TenantID:  tenantID,
				ModelUID:  inst.ModelUID,
				AssetID:   inst.AssetID,
				AssetName: inst.AssetName,
				IssueType: w.Type,
				FieldUID:  w.Field,
				Message:   w.Message,
			}
			if s.syncQualityIssueRepo == nil {
				s.logger.Warn("同步质量问题（未持久化，repo 为空）",
					elog.Int64("tenant_id", tenantID),
					elog.String("asset_id", inst.AssetID),
					elog.String("field", w.Field),
					elog.String("type", w.Type))
				continue
			}
			if _, err := s.syncQualityIssueRepo.UpsertByKey(ctx, issue); err != nil {
				s.logger.Warn("同步质量问题写入失败",
					elog.String("asset_id", inst.AssetID),
					elog.String("field", w.Field),
					elog.FieldErr(err))
			}
		}
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

	id, err := s.repo.Create(ctx, instance)
	if err != nil {
		return 0, err
	}

	// 创建审计：所有属性视为「新增」（OldValue 为空）
	s.recordChanges(ctx, changeMetaFromInstance(instance, domain.ChangeSourceManual), nil, instance.Attributes)
	return id, nil
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

	// 变更审计需要旧值做 diff，先读旧实例（失败不阻断更新，只是无审计）
	var old domain.Instance
	if s.changeHistoryRepo != nil {
		if o, err := s.repo.GetByID(ctx, tenantID, instance.ID); err == nil {
			old = o
		}
	}

	// 更新：只做**非阻断**校验（blockRequired=false）。
	// 存量 4132 条实例已缺失必填字段（如 ecs 的 instance_id/instance_name 在真实数据中
	// 从不存在，数据用 asset_id/asset_name）。若更新也硬拦，这些历史实例将无法再编辑，
	// 属明显回归。产品已确认「只对新写入生效」，故此处仅记录警告。
	if err := s.validateModelAttributes(ctx, instance, false); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, tenantID, instance); err != nil {
		return err
	}

	// 更新审计：diff 旧属性 vs 新属性
	if old.ID != 0 {
		s.recordChanges(ctx, changeMetaFromInstance(instance, domain.ChangeSourceManual), old.Attributes, instance.Attributes)
	}
	return nil
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

// Delete 删除实例，并**级联解除**其参与的所有关系。
//
// 缺陷修复（实测 2026-11-02）：此前只删实例、完全不动关系。
// 实测复现：建 A→B 关系后删除 B，关系仍残留（target 指向已不存在的实例），
// 即产生**悬空关系**。这违反了已确认的关系语义规则第 5 条
// 「删除资产时级联解除关系」。
//
// 顺序：先删实例、再清关系。
// 理由：关系清理失败不应阻止删除本身（否则脏数据永远删不掉）；
// 清理失败只记警告，且该关系会因目标实例已不存在而在后续体检中暴露。
func (s *instanceService) Delete(ctx context.Context, tenantID int64, id int64) error {
	existing, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrInstanceNotFound
	}

	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}

	s.detachRelations(ctx, tenantID, id)
	s.cleanupSyncQualityIssues(ctx, tenantID, existing.AssetID)
	// 删除审计：所有字段视为「被删除」（NewValue 为空）
	s.recordChanges(ctx, changeMetaFromInstance(existing, domain.ChangeSourceManual), existing.Attributes, nil)
	return nil
}

// cleanupSyncQualityIssues 删除实例后，级联清理其同步质量问题记录。
//
// 背景（实测 2026-11-02）：同步问题清单按 (tenant, asset_id, ...) 去重，
// 实例删除后若不清理，会留下指向已不存在实例的「悬空问题」，
// 污染清单、误导排查。与 detachRelations 同构，均为删除实例时的级联清理。
func (s *instanceService) cleanupSyncQualityIssues(ctx context.Context, tenantID int64, assetID string) {
	if s.syncQualityIssueRepo == nil || assetID == "" {
		return
	}
	if err := s.syncQualityIssueRepo.DeleteByAssetID(ctx, tenantID, assetID); err != nil {
		s.logger.Warn("级联清理同步问题记录失败",
			elog.Int64("tenant_id", tenantID),
			elog.String("asset_id", assetID),
			elog.FieldErr(err))
	}
}

// changeMetaFromInstance 从实例上下文提取变更元数据。
func changeMetaFromInstance(inst domain.Instance, source string) domain.ChangeMetadata {
	meta := domain.ChangeMetadata{
		AssetID:      inst.AssetID,
		AssetName:    inst.AssetName,
		ModelUID:     inst.ModelUID,
		TenantID:     inst.TenantID,
		AccountID:    inst.AccountID,
		ChangeSource: source,
	}
	if inst.Attributes != nil {
		meta.Provider = toStr(inst.Attributes["provider"])
		meta.Region = toStr(inst.Attributes["region"])
	}
	return meta
}

func toStr(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// recordChanges 把字段变更（变更前后值）写入 ecam_change_history。
//
// 需求（2026-11-02 产品确认）：审计需覆盖「变更前后值」。与 e-cam 共享
// 同一集合，供现有 /api/v1/cam/audit/changes 面板消费。
//
// 非阻断：审计写入失败只记日志，绝不影响业务操作（审计挂了不能导致 CRUD 挂）。
// oldAttrs / newAttrs 为 nil 分别表示「创建」（全新增）与「删除」（全移除）。
func (s *instanceService) recordChanges(ctx context.Context, meta domain.ChangeMetadata, oldAttrs, newAttrs map[string]interface{}) {
	if s.changeHistoryRepo == nil {
		return
	}
	changes := domain.DiffAttributes(oldAttrs, newAttrs)
	if len(changes) == 0 {
		return
	}
	now := time.Now().UnixMilli()
	records := make([]domain.ChangeRecord, len(changes))
	for i, c := range changes {
		records[i] = domain.ChangeRecord{
			AssetID:      meta.AssetID,
			AssetName:    meta.AssetName,
			ModelUID:     meta.ModelUID,
			TenantID:     meta.TenantID,
			AccountID:    meta.AccountID,
			Provider:     meta.Provider,
			Region:       meta.Region,
			FieldName:    c.FieldName,
			OldValue:     c.OldValue,
			NewValue:     c.NewValue,
			ChangeSource: meta.ChangeSource,
			Ctime:        now,
		}
	}
	if err := s.changeHistoryRepo.BatchCreate(ctx, records); err != nil {
		s.logger.Warn("审计变更记录写入失败",
			elog.String("asset_id", meta.AssetID),
			elog.Int("count", len(records)),
			elog.FieldErr(err))
	}
}

// detachRelations 级联解除某实例作为源或目标的全部关系。
func (s *instanceService) detachRelations(ctx context.Context, tenantID int64, instanceID int64) {
	if s.relationRepo == nil {
		s.logger.Warn("relationRepo 未注入，跳过关系级联解除",
			elog.Int64("tenant_id", tenantID), elog.Int64("instance_id", instanceID))
		return
	}
	if err := s.relationRepo.DeleteByInstanceID(ctx, tenantID, instanceID); err != nil {
		// 不阻断删除主流程，仅记录：否则脏数据将永远无法删除
		s.logger.Warn("级联解除实例关系失败",
			elog.Int64("tenant_id", tenantID),
			elog.Int64("instance_id", instanceID),
			elog.FieldErr(err))
		return
	}
	s.logger.Info("已级联解除实例关系",
		elog.Int64("tenant_id", tenantID), elog.Int64("instance_id", instanceID))
}

// DeleteByAccountID 按云账号批量删除实例。
//
// 该路径删除的是**一批**实例，若不清理关系会一次性产生大量悬空关系，
// 危害比单条删除更大。因此先取出待删实例 ID，删除后再逐个解除关系。
func (s *instanceService) DeleteByAccountID(ctx context.Context, accountID int64) error {
	// 先收集受影响实例（用于清理关系）；取不到不影响删除本身
	var ids []int64
	if s.relationRepo != nil {
		if list, err := s.repo.List(ctx, domain.InstanceFilter{AccountID: accountID, Limit: 100000}); err != nil {
			s.logger.Warn("按账号删除前查询实例列表失败，将跳过关系清理",
				elog.Int64("account_id", accountID), elog.FieldErr(err))
		} else {
			for _, inst := range list {
				ids = append(ids, inst.ID)
			}
		}
	}

	if err := s.repo.DeleteByAccountID(ctx, accountID); err != nil {
		return err
	}

	for _, id := range ids {
		s.detachRelations(ctx, 0, id)
	}
	return nil
}

func (s *instanceService) Upsert(ctx context.Context, instance domain.Instance) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	// 方案 B（记录但不阻断）：单条 upsert 与批量一致，先 warn-only 校验并记录问题清单。
	// 之前单条 Upsert 漏接，导致走单条导入/同步的实例绕过了字段问题记录。
	s.recordSyncQualityIssues(ctx, instance.TenantID, []domain.Instance{instance})
	return s.repo.Upsert(ctx, instance)
}

func (s *instanceService) UpsertBatch(ctx context.Context, instances []domain.Instance) error {
	// 方案 B（记录但不阻断）：先做一轮 warn-only 字段校验并记录问题清单。
	// 注意：必须在写库**之前**采集 tenantID，且不因记录失败而中断后续写入。
	if len(instances) > 0 {
		s.recordSyncQualityIssues(ctx, instances[0].TenantID, instances)
	}

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
