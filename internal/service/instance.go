package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gotomicro/ego/core/elog"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
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
	// BatchDelete 批量删除实例（逐条结果，不回滚；每条复用单条 Delete 的级联+审计）。
	BatchDelete(ctx context.Context, tenantID int64, ids []int64) (domain.BatchDeleteResult, error)
	Upsert(ctx context.Context, instance domain.Instance) error
	UpsertBatch(ctx context.Context, instances []domain.Instance) error
	// RequiredAttributeNames 返回模型必填字段的 field_uid → 展示名，
	// 供「编辑时清空字段」判断哪些字段不允许被清空（2026-11-18 产品确认）。
	RequiredAttributeNames(ctx context.Context, modelUID string) (map[string]string, error)
	// Facets 聚合实例列表的筛选可选值（provider / region / source）。
	Facets(ctx context.Context, tenantID int64) (*domain.InstanceFacets, error)
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

// RequiredAttributeNames 返回模型必填字段的 field_uid → 展示名。
//
// 用途：编辑实例时把某个属性显式置空（清空）属于**破坏性**语义，
// 必填字段必须被拒绝。这里只读取定义，不做写入。
// attrRepo 为 nil 或拉取失败时返回空集合 + error，由调用方决定降级策略
// （handler 侧对 error 采取「放行并记日志」，避免附属查询故障阻断编辑）。
func (s *instanceService) RequiredAttributeNames(ctx context.Context, modelUID string) (map[string]string, error) {
	if s.attrRepo == nil || modelUID == "" {
		return nil, nil
	}
	defs, err := s.attrRepo.List(ctx, domain.AttributeFilter{ModelUID: modelUID, Limit: 500})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(defs))
	for _, d := range defs {
		if !d.Required || d.FieldUID == "" {
			continue
		}
		name := d.DisplayName
		if name == "" {
			name = d.FieldName
		}
		out[d.FieldUID] = name
	}
	return out, nil
}

// Facets 聚合实例列表的筛选可选值（provider / region / source）。
//
// 只做透传：聚合在 DAO 完成（一次 $facet，实测 ~120ms），
// 服务层不再二次过滤，避免前端看到的口径与列表查询不一致。
func (s *instanceService) Facets(ctx context.Context, tenantID int64) (*domain.InstanceFacets, error) {
	if tenantID <= 0 {
		return &domain.InstanceFacets{}, nil
	}
	return s.repo.Facets(ctx, tenantID)
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

// BatchItemError 表示批量操作中第 Index 条（0 基）失败。
//
// 批量入口此前只返回一个笼统错误（甚至把 Mongo 的 E11000 原文透传给调用方），
// 调用方无法定位是哪一条、哪个资产。批量能力必须能逐条定位，否则「部分失败」
// 无法重试——这直接对应「批量处理采用全量回滚还是逐条结果」的语义要求。
type BatchItemError struct {
	Index    int    // 0 基下标
	ModelUID string // 失败条目的模型（便于定位）
	AssetID  string // 失败条目的资产 ID
	Err      error  // 根因（可用 errors.Is / errors.As 判断）
}

func (e *BatchItemError) Error() string {
	return fmt.Sprintf("第 %d 条（model_uid=%s, asset_id=%s）：%v", e.Index+1, e.ModelUID, e.AssetID, e.Err)
}

func (e *BatchItemError) Unwrap() error { return e.Err }

// validateCreateOne 是单条创建与批量创建共用的校验序列。
//
// 顺序与历史行为保持一致：字段完整性 → 模型存在 → 必填字段 → 重复检查。
// 单条与批量必须走同一套，否则 /instances/batch 会绕开必填校验
// （实测 2026-11-19：ecs 缺 6 个必填字段的条目经批量入口返回 code=0 并落库）。
func (s *instanceService) validateCreateOne(ctx context.Context, instance *domain.Instance) error {
	instance.Normalize()
	if err := instance.Validate(); err != nil {
		return err
	}

	model, err := s.modelRepo.GetByUID(ctx, instance.ModelUID)
	if err != nil {
		return fmt.Errorf("failed to check model existence: %w", err)
	}
	if model.ID == 0 {
		return errs.ErrModelNotFound
	}

	// 新建：按模型字段定义校验，必填缺失直接拒绝
	if err := s.validateModelAttributes(ctx, *instance, true); err != nil {
		return err
	}

	existing, err := s.repo.GetByAssetID(ctx, instance.TenantID, instance.ModelUID, instance.AssetID)
	if err != nil {
		return fmt.Errorf("failed to check existing instance: %w", err)
	}
	if existing.ID > 0 {
		return errs.ErrInstanceExists
	}

	return nil
}

func (s *instanceService) Create(ctx context.Context, instance domain.Instance) (int64, error) {
	if err := s.validateCreateOne(ctx, &instance); err != nil {
		return 0, err
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

	// 批量与单条共用同一套校验（模型存在性、必填字段、重复检查）。
	// 历史实现只做 inst.Validate()，缺必填字段的条目可以经批量入口直接落库。
	// 批内重复（同一 model_uid + asset_id 出现两次）同样要拦：唯一索引会以
	// E11000 拒绝整批，而调用方拿不到「是第几条冲突」。
	seen := make(map[string]struct{}, len(instances))
	for i := range instances {
		// 就地归一化：校验与最终写库（repo.CreateBatch(instances)）必须是同一份数据，
		// 否则 asset_id 首尾空格会在校验时被裁掉、写入时又带回去。
		inst := &instances[i]
		if err := s.validateCreateOne(ctx, inst); err != nil {
			return 0, &BatchItemError{Index: i, ModelUID: inst.ModelUID, AssetID: inst.AssetID, Err: err}
		}
		key := fmt.Sprintf("%d|%s|%s", inst.TenantID, inst.ModelUID, inst.AssetID)
		if _, dup := seen[key]; dup {
			return 0, &BatchItemError{
				Index:    i,
				ModelUID: inst.ModelUID,
				AssetID:  inst.AssetID,
				Err:      fmt.Errorf("%w: 同一批次内重复出现", errs.ErrInstanceExists),
			}
		}
		seen[key] = struct{}{}
	}

	count, err := s.repo.CreateBatch(ctx, instances)
	if err != nil {
		// 有序批量插入可能在中途失败：前面若干条已经落库，只是调用方拿到错误。
		// 这些已落库的条目同样要有创建审计（否则正是本次要补的缺口），因此按
		// repo 返回的实际写入条数补写前缀，再把错误原样上抛。
		if count > 0 && count <= int64(len(instances)) {
			s.recordCreationChanges(ctx, instances[:count])
		}
		return count, err
	}

	// 创建审计：批量入口过去只写库、从不写审计，导致「同一批资产批量建了」在
	// ecam_change_history 里完全查不到，与单条 Create / 导入 upsert 的口径不一致
	// （同类审计缺口见 2026-11-02 导入通路修复）。写库成功后才写审计，且审计失败
	// 只记日志（非阻断）：审计挂了不能让已入库的数据看起来像失败。
	s.recordCreationChanges(ctx, instances)
	return count, nil
}

// recordCreationChanges 为批量创建的实例补写「创建」审计（全部字段视为新增）。
//
// 与单条 Create 的差异只在写入方式：这里把所有实例的字段变更合并成一次批量写，
// 并按 changeRecordChunkSize 切块，避免一次请求携带过多文档（批量上限即批次上限）。
func (s *instanceService) recordCreationChanges(ctx context.Context, instances []domain.Instance) {
	if s.changeHistoryRepo == nil || len(instances) == 0 {
		return
	}
	operator := middleware.OperatorFrom(ctx)
	now := time.Now().UnixMilli()

	pending := make([]domain.ChangeRecord, 0, len(instances))
	// chunkIndex 从 1 开始，用于把失败定位到具体切块：日志里的 chunk_record_count 只描述
	// 这一块，而 batch_instance_count 描述整个批次，两者不能混用同一字段名（评审 #1）。
	chunkIndex := 0
	flush := func() {
		if len(pending) == 0 {
			return
		}
		chunkIndex++
		if err := s.changeHistoryRepo.BatchCreate(ctx, pending); err != nil {
			s.logger.Warn("批量创建审计变更记录写入失败",
				elog.Int("batch_instance_count", len(instances)),
				elog.Int("chunk_index", chunkIndex),
				elog.Int("chunk_record_count", len(pending)),
				elog.FieldErr(err))
		}
		pending = pending[:0]
	}

	for _, inst := range instances {
		meta := changeMetaFromInstance(inst, domain.ChangeSourceManual)
		pending = append(pending, buildChangeRecords(meta, domain.DiffAttributes(nil, inst.Attributes), operator, now)...)
		if len(pending) >= changeRecordChunkSize {
			flush()
		}
	}
	flush()
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

// BatchDelete 批量删除实例。
//
// 语义（2026-11-02 关系语义第 6 条「逐条结果，不回滚」扩展到实例）：
//   - 逐条调用单条 Delete（复用级联解除关系 + 审计 + 清理同步问题记录）
//   - 单条失败不中断后续，成功与失败分别收集返回
//   - 不做全量回滚：部分失败时已删除的不会恢复
//
// 安全边界：调用方（web 层）必须传入**已按租户校验**的 ids；
// 本方法对每条仍走带 tenantID 的 Delete，天然防跨租户误删。
func (s *instanceService) BatchDelete(ctx context.Context, tenantID int64, ids []int64) (domain.BatchDeleteResult, error) {
	result := domain.BatchDeleteResult{
		Deleted: make([]int64, 0, len(ids)),
		Failed:  make([]domain.BatchDeleteItem, 0),
		Total:   len(ids),
	}
	for _, id := range ids {
		if err := s.Delete(ctx, tenantID, id); err != nil {
			result.Failed = append(result.Failed, domain.BatchDeleteItem{ID: id, Error: err.Error()})
			continue
		}
		result.Deleted = append(result.Deleted, id)
	}
	return result, nil
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

// changeRecordChunkSize 是单次 BatchCreate 写入的变更记录上限。
//
// 批量创建 1000 条实例、每条 10 个字段就是 1 万条审计；一次性下发会让单个请求体
// 过大（也更容易被 Mongo 的 16MB 文档/内存限制打到），因此按块切分。
const changeRecordChunkSize = 500

// buildChangeRecords 把一组字段变更映射成审计记录。
//
// 单条写（recordChanges）与批量创建（recordCreationChanges）共用这一处映射：
// ChangeRecord 的字段一旦增减，两条路径必须同时生效，否则同一个操作用不同入口
// 进来的审计口径就会分叉。
func buildChangeRecords(meta domain.ChangeMetadata, changes []domain.FieldChange, operator string, now int64) []domain.ChangeRecord {
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
			Operator:     operator,
			Ctime:        now,
		}
	}
	return records
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
	records := buildChangeRecords(meta, changes, middleware.OperatorFrom(ctx), time.Now().UnixMilli())
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

// instanceAccountEndpoint 是「按账号删除」时待清理关系的实例端点。
//
// 关系仓储的 DeleteByInstanceID 按 `tenant_id` 精确匹配，
// 所以解绑必须带上实例自身的租户；只带实例 ID 会退化成 tenant 0 而删不掉任何关系。
type instanceAccountEndpoint struct {
	tenantID   int64
	instanceID int64
}

// DeleteByAccountID 按云账号批量删除实例。
//
// 该路径删除的是**一批**实例，若不清理关系会一次性产生大量悬空关系，
// 危害比单条删除更大。因此先取出待删实例 ID，删除后再逐个解除关系。
func (s *instanceService) DeleteByAccountID(ctx context.Context, accountID int64) error {
	// 先收集受影响实例（用于清理关系）；取不到不影响删除本身。
	//
	// 必须连同实例自身的 tenant_id 一起收集：关系仓储按 `tenant_id + 实例 ID`
	// 精确匹配删除（见 dao.DeleteByInstanceID），传 0 会一条都删不掉。
	// 背景（实测 2026-11-24）：此前这里硬编码 detachRelations(ctx, 0, id)，
	// 于是「按账号删除实例」永远不会清理关系 —— 生产上已观察到悬空关系
	// relation_id=400（source=1965 存在、target=385609 已不存在），
	// 且质量页 dangling_relation_count 由此从 0 变 1。
	var endpoints []instanceAccountEndpoint
	if s.relationRepo != nil {
		endpoints = s.collectAccountEndpoints(ctx, accountID)
	}

	if err := s.repo.DeleteByAccountID(ctx, accountID); err != nil {
		return err
	}

	for _, ep := range endpoints {
		s.detachRelations(ctx, ep.tenantID, ep.instanceID)
	}
	return nil
}

// accountDetachPageSize 是「按账号删除」收集待解绑实例的分页大小。
//
// 不能用一次 `Limit: 100000` 了事：`DeleteByAccountID` 本身按 account_id 无上限删除，
// 而 List 带 limit 时只返回前 N 条，超出部分被删掉却不会被解绑 —— 又留下一批悬空关系，
// 且日志里看不出异常（正是本缺陷要消除的那类静默残留）。
const accountDetachPageSize = 5000

// accountDetachMaxPages 是分页收集的安全上限（1,000 万实例），仅用于防死循环。
const accountDetachMaxPages = 2000

// collectAccountEndpoints 分页收集某云账号下所有实例的 (tenant_id, instance_id)。
//
// 中途查询失败时保留已收集到的端点：这些实例同样会被删除，先把已知关系清掉，
// 比整批放弃更接近「不留悬空关系」的目标；缺口通过 Warn 暴露。
func (s *instanceService) collectAccountEndpoints(ctx context.Context, accountID int64) []instanceAccountEndpoint {
	var endpoints []instanceAccountEndpoint
	for page := 0; page < accountDetachMaxPages; page++ {
		offset := int64(page) * accountDetachPageSize
		list, err := s.repo.List(ctx, domain.InstanceFilter{
			AccountID: accountID,
			Offset:    offset,
			Limit:     accountDetachPageSize,
		})
		if err != nil {
			s.logger.Warn("按账号删除前查询实例列表失败，关系清理将只覆盖已收集到的实例",
				elog.Int64("account_id", accountID),
				elog.Int64("collected", int64(len(endpoints))),
				elog.FieldErr(err))
			return endpoints
		}
		for _, inst := range list {
			endpoints = append(endpoints, instanceAccountEndpoint{tenantID: inst.TenantID, instanceID: inst.ID})
		}
		if len(list) < accountDetachPageSize {
			return endpoints
		}
	}
	s.logger.Warn("按账号删除前收集实例达到分页上限，可能存在关系清理盲区",
		elog.Int64("account_id", accountID),
		elog.Int64("collected", int64(len(endpoints))),
		elog.Int("max_pages", accountDetachMaxPages))
	return endpoints
}

func (s *instanceService) Upsert(ctx context.Context, instance domain.Instance) error {
	instance.Normalize()
	if err := instance.Validate(); err != nil {
		return err
	}
	// 方案 B（记录但不阻断）：单条 upsert 与批量一致，先 warn-only 校验并记录问题清单。
	// 之前单条 Upsert 漏接，导致走单条导入/同步的实例绕过了字段问题记录。
	s.recordSyncQualityIssues(ctx, instance.TenantID, []domain.Instance{instance})

	// 写库前先取旧值：upsert 是「有则更新、无则新增」，变更审计需要旧属性才能算出 diff。
	existing, hadExisting := s.snapshotBeforeUpsert(ctx, instance)
	if err := s.repo.Upsert(ctx, instance); err != nil {
		return err
	}
	s.recordUpsertChanges(ctx, instance, existing, hadExisting)
	return nil
}

// UpsertBatchError 描述批量写入在第几条失败，以及此前已成功写入的条数。
//
// 批量写入沿用「首错中止」语义：某一条写库失败后不再继续，也不回滚已写入的数据
// （逐条 upsert 没有事务）。因此调用方要能正确恢复，必须同时知道两件事：
//  1. 是哪一条失败、原因是什么；
//  2. 这次请求到底已经写进去几条。
//
// 修复前这里直接返回底层错误（如 "asset id cannot be empty"），两者都拿不到：
// 导入场景下用户只看到「导入失败」，而库里其实已经多出若干条记录。
type UpsertBatchError struct {
	Index    int // 失败条目在请求中的下标（0 起）
	ModelUID string
	AssetID  string
	Written  int // 本次请求中此条之前已成功写入的条数
	Total    int // 本次请求的条目总数
	Err      error
}

func (e *UpsertBatchError) Error() string {
	return fmt.Sprintf("第 %d 条（asset_id=%s）写入失败：%s；本次请求共 %d 条，已写入 %d 条，其余未处理",
		e.Index+1, e.AssetID, DescribeInstanceWriteError(e.Err), e.Total, e.Written)
}

// Unwrap 让调用方仍可用 errors.Is 判断根因（例如 errs.ErrInvalidAssetID）。
func (e *UpsertBatchError) Unwrap() error { return e.Err }

// DescribeInstanceWriteError 把写入链路里的英文哨兵错误翻成面向用户的中文。
//
// 未识别的错误原样返回：仓储层错误（Mongo 超时等）本来就需要运维视角的原文，
// 不该被这里误译成业务话术。
func DescribeInstanceWriteError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errs.ErrInvalidAssetID):
		return "资产ID不能为空"
	case errors.Is(err, errs.ErrInvalidModelUID):
		return "模型不能为空"
	case errors.Is(err, errs.ErrInvalidTenantID):
		return "租户信息缺失，请重新登录后重试"
	case errors.Is(err, errs.ErrInvalidAttributes):
		return "字段校验未通过"
	}
	return err.Error()
}

// IsInstanceWriteValidationError 判断是否属于「调用方数据问题」而非服务端故障。
//
// 用于让单条写入失败也返回 400 族状态码：修复前所有写入错误都被包成
// 500001（系统错误），调用方无法区分「我传错了」和「服务端挂了」。
func IsInstanceWriteValidationError(err error) bool {
	if err == nil {
		return false
	}
	var batchErr *UpsertBatchError
	if errors.As(err, &batchErr) {
		return IsInstanceWriteValidationError(batchErr.Err)
	}
	return errors.Is(err, errs.ErrInvalidAssetID) ||
		errors.Is(err, errs.ErrInvalidModelUID) ||
		errors.Is(err, errs.ErrInvalidTenantID) ||
		errors.Is(err, errs.ErrInvalidAttributes)
}

func (s *instanceService) UpsertBatch(ctx context.Context, instances []domain.Instance) error {
	// 方案 B（记录但不阻断）：先做一轮 warn-only 字段校验并记录问题清单。
	// 注意：必须在写库**之前**采集 tenantID，且不因记录失败而中断后续写入。
	if len(instances) > 0 {
		s.recordSyncQualityIssues(ctx, instances[0].TenantID, instances)
	}

	total := len(instances)
	written := 0
	for i, inst := range instances {
		inst.Normalize()
		// 失败时把「第几条 + 已写入几条」一并带出：首错中止不回滚，
		// 调用方不知道已写入数量就无法判断该重试整批还是从断点续传。
		fail := func(err error) error {
			return &UpsertBatchError{
				Index:    i,
				ModelUID: inst.ModelUID,
				AssetID:  inst.AssetID,
				Written:  written,
				Total:    total,
				Err:      err,
			}
		}
		if err := inst.Validate(); err != nil {
			return fail(err)
		}
		existing, hadExisting := s.snapshotBeforeUpsert(ctx, inst)
		if err := s.repo.Upsert(ctx, inst); err != nil {
			s.logger.Error("failed to upsert instance",
				elog.String("asset_id", inst.AssetID),
				elog.String("model_uid", inst.ModelUID),
				elog.Int("already_written", written),
				elog.FieldErr(err),
			)
			return fail(err)
		}
		s.recordUpsertChanges(ctx, inst, existing, hadExisting)
		written++
	}
	return nil
}

// snapshotBeforeUpsert 读取 upsert 之前的实例快照，供变更审计算 diff。
//
// 仅在注入审计仓储时才查询（省一次读）；查询失败视为「库里没有」，
// 既不影响写入，也不阻断审计之外的任何流程。
func (s *instanceService) snapshotBeforeUpsert(ctx context.Context, instance domain.Instance) (domain.Instance, bool) {
	if s.changeHistoryRepo == nil {
		return domain.Instance{}, false
	}
	old, err := s.repo.GetByAssetID(ctx, instance.TenantID, instance.ModelUID, instance.AssetID)
	if err != nil || old.AssetID == "" {
		return domain.Instance{}, false
	}
	return old, true
}

// recordUpsertChanges 为导入（upsert）路径补写变更审计。
//
// 背景（2026-11-19）：Create / Update / Delete 都会写 ecam_change_history，
// 唯独 Upsert / UpsertBatch 不写。而 e-cmdb 的 upsert 唯一调用方是「导入实例」，
// 于是通过导入写进来的资产在「变更审计」里完全不可见（实测 changes total=0），
// 前端审计面板却早已把 import 映射成「导入」标签 —— 说明这是漏接，不是设计。
// 非阻断：审计写入失败只记日志（recordChanges 内部处理）。
func (s *instanceService) recordUpsertChanges(ctx context.Context, inst, existing domain.Instance, hadExisting bool) {
	meta := changeMetaFromInstance(inst, domain.ChangeSourceImport)
	if !hadExisting {
		// 新增：与 Create 口径一致，全部字段记为新值。
		s.recordChanges(ctx, meta, nil, inst.Attributes)
		return
	}
	s.recordChanges(ctx, meta, existing.Attributes, inst.Attributes)
}
