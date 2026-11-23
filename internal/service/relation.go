package service

import (
	"context"
	"fmt"

	"github.com/gotomicro/ego/core/elog"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// RelationService 实例关系服务接口
type RelationService interface {
	Create(ctx context.Context, relation domain.InstanceRelation) (int64, error)
	CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error)
	GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error)
	List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, int64, error)
	// ListDetailed 返回带两端实例摘要（asset_id/asset_name/model_uid）的关系列表，
	// 供列表页直接展示，避免前端按 ID 逐条二次查询。
	ListDetailed(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelationDetail, int64, error)
	Delete(ctx context.Context, tenantID int64, id int64) error
	DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error
	// DeriveRelations 按已确认的规则从既有实例属性推导实例关系。
	// dryRun=true 时只统计不写入，便于在执行前核对影响面。
	DeriveRelations(ctx context.Context, tenantID int64, rule string, dryRun bool) (domain.DeriveResult, error)
}

type relationService struct {
	repo repository.InstanceRelationRepository
	// instanceRepo 用于推导关系时读取实例（按租户过滤）
	instanceRepo repository.InstanceRepository
	// modelRelRepo 用于读取关系类型声明的源/目标模型，创建前校验模型对
	modelRelRepo repository.ModelRelationTypeRepository
	logger       *elog.Component
}

// NewRelationService 创建关系服务
func NewRelationService(repo repository.InstanceRelationRepository, instanceRepo repository.InstanceRepository, modelRelRepo repository.ModelRelationTypeRepository) RelationService {
	return &relationService{
		repo:         repo,
		instanceRepo: instanceRepo,
		modelRelRepo: modelRelRepo,
		logger:       elog.DefaultLogger,
	}
}

// loadEndpointModels 加载关系两端实例的模型 UID（同租户），同时校验每个 id 都存在。
//
// 缺陷背景（实测 2026-11-17）：Create 只做查重、不校验端点实例，历史上产生了
// 8 条两端皆亡的悬空关系（数据质量页 dangling_relation）。删除实例虽已级联解除
// 关系，但通过 API/批量推送仍可制造新的悬空引用，故在写入前拦截。
func (s *relationService) loadEndpointModels(ctx context.Context, tenantID int64, ids []int64) (map[int64]string, error) {
	uniq := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	instances, err := s.instanceRepo.ListByIDs(ctx, uniq, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to check instances existence: %w", err)
	}
	models := make(map[int64]string, len(instances))
	for _, inst := range instances {
		models[inst.ID] = inst.ModelUID
	}
	for _, id := range uniq {
		if _, ok := models[id]; !ok {
			return nil, fmt.Errorf("%w: instance_id=%d", errs.ErrInstanceNotFound, id)
		}
	}
	return models, nil
}

// validateRelationEndpoints 校验关系两端实例存在（同租户）且模型符合关系类型声明。
//
// 缺陷背景（实测 2026-11-18）：Create/CreateBatch 此前只校验端点是否存在，
// 不校验模型对，于是可以用 vpc_subnet（声明 vpc → subnet）把两个 host 实例连起来，
// 关系的方向与基数语义因此不成立，拓扑与影响分析会给出错误结论。
// 实测现存 1304 条实例关系全部符合各自关系类型声明的模型对，说明该校验与既有数据兼容。
func (s *relationService) validateRelationEndpoints(ctx context.Context, relations []domain.InstanceRelation) error {
	if len(relations) == 0 {
		return nil
	}
	// 模型对校验依赖端点实例的 model_uid，因此 instanceRepo 缺失时两种校验都无从进行，
	// 统一在这里跳过（原先 modelRelRepo 的 nil 兜底散落在 validateModelPair 里，
	// 一旦 instanceRepo 为 nil 而 modelRelRepo 非 nil，就会拿空 models 表比对，
	// 得到 actual=-> 的 mismatch 并拒绝一切写入）。
	if s.instanceRepo == nil {
		return nil
	}
	idsByTenant := make(map[int64][]int64, 4)
	for _, rel := range relations {
		idsByTenant[rel.TenantID] = append(idsByTenant[rel.TenantID], rel.SourceInstanceID, rel.TargetInstanceID)
	}
	typeCache := make(map[string]domain.ModelRelationType, len(relations))
	for tenantID, ids := range idsByTenant {
		models, err := s.loadEndpointModels(ctx, tenantID, ids)
		if err != nil {
			return err
		}
		// modelRelRepo 未注入时只保留端点存在性校验，兼容旧装配与既有单测。
		if s.modelRelRepo == nil {
			continue
		}
		for _, rel := range relations {
			if rel.TenantID != tenantID {
				continue
			}
			if err := s.validateModelPair(ctx, rel, models, typeCache); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateModelPair 校验单条关系两端的模型与关系类型声明一致。
// 调用方（validateRelationEndpoints）已保证 instanceRepo 与 modelRelRepo 均非 nil，
// 且 models 覆盖本条关系的两端实例，这里不再做 nil 兜底，避免两处条件漂移。
func (s *relationService) validateModelPair(
	ctx context.Context,
	rel domain.InstanceRelation,
	models map[int64]string,
	cache map[string]domain.ModelRelationType,
) error {
	relType, ok := cache[rel.RelationTypeUID]
	if !ok {
		loaded, err := s.modelRelRepo.GetByUID(ctx, rel.RelationTypeUID)
		if err != nil {
			return fmt.Errorf("failed to load relation type %q: %w", rel.RelationTypeUID, err)
		}
		if loaded.ID == 0 {
			return fmt.Errorf("%w: relation_type_uid=%s", errs.ErrRelationTypeNotFound, rel.RelationTypeUID)
		}
		relType = loaded
		cache[rel.RelationTypeUID] = loaded
	}
	sourceModel := models[rel.SourceInstanceID]
	targetModel := models[rel.TargetInstanceID]
	if sourceModel == relType.SourceModelUID && targetModel == relType.TargetModelUID {
		return nil
	}
	// 批量创建时整体拒绝，必须能定位到具体条目，因此带上两端 instance_id。
	return fmt.Errorf("%w: relation_type=%s source_instance_id=%d target_instance_id=%d declared=%s->%s actual=%s->%s",
		errs.ErrRelationModelMismatch, rel.RelationTypeUID, rel.SourceInstanceID, rel.TargetInstanceID,
		relType.SourceModelUID, relType.TargetModelUID, sourceModel, targetModel)
}

func (s *relationService) Create(ctx context.Context, relation domain.InstanceRelation) (int64, error) {
	if err := s.validateRelationEndpoints(ctx, []domain.InstanceRelation{relation}); err != nil {
		return 0, err
	}
	exists, err := s.repo.Exists(ctx, relation.TenantID, relation.SourceInstanceID, relation.TargetInstanceID, relation.RelationTypeUID)
	if err != nil {
		return 0, fmt.Errorf("failed to check relation existence: %w", err)
	}
	if exists {
		return 0, errs.ErrRelationExists
	}

	return s.repo.Create(ctx, relation)
}

func (s *relationService) CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error) {
	if len(relations) == 0 {
		return 0, nil
	}
	// 校验端点存在性与模型对（按租户聚合）；全部通过才写入，避免部分写入产生非法关系
	if err := s.validateRelationEndpoints(ctx, relations); err != nil {
		return 0, err
	}
	return s.repo.CreateBatch(ctx, relations)
}

func (s *relationService) GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error) {
	relation, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return domain.InstanceRelation{}, fmt.Errorf("failed to get relation: %w", err)
	}
	if relation.ID == 0 {
		return domain.InstanceRelation{}, errs.ErrRelationNotFound
	}
	return relation, nil
}

func (s *relationService) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, int64, error) {
	relations, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list relations: %w", err)
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count relations: %w", err)
	}

	return relations, total, nil
}

// ListDetailed 返回带两端实例摘要的关系列表。
//
// 背景（2026-11-02）：实例关系列表此前只返回 source/target 的数字 ID，
// 前端要展示「哪台 ecs 挂了哪些盘」必须按 ID 逐条二次查询实例。
// 实测 1312 条关系涉及 2020 个去重实例 ID，逐条查询不可接受。
// 这里用 instanceRepo.ListByIDs 一次性批量解析两端实例，拼出摘要。
func (s *relationService) ListDetailed(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelationDetail, int64, error) {
	relations, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list relations: %w", err)
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count relations: %w", err)
	}

	// 收集本页所有端点 ID 去重，一次批量解析
	ids := make([]int64, 0, len(relations)*2)
	seen := make(map[int64]bool, len(relations)*2)
	for _, r := range relations {
		for _, id := range []int64{r.SourceInstanceID, r.TargetInstanceID} {
			if id > 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}

	refs := make(map[int64]domain.InstanceRef, len(ids))
	if len(ids) > 0 {
		instances, lerr := s.instanceRepo.ListByIDs(ctx, ids, filter.TenantID)
		if lerr != nil {
			// 解析失败不阻断列表展示：退化为「只有 ID、无摘要」，
			// 前端仍可正常分页，只是资产名显示为 ID
			s.logger.Warn("批量解析关系端点实例失败",
				elog.Int("ids", len(ids)), elog.FieldErr(lerr))
			instances = nil
		}
		for _, inst := range instances {
			refs[inst.ID] = domain.InstanceRef{
				ID:        inst.ID,
				AssetID:   inst.AssetID,
				AssetName: inst.AssetName,
				ModelUID:  inst.ModelUID,
			}
		}
	}

	details := make([]domain.InstanceRelationDetail, len(relations))
	for i, r := range relations {
		details[i] = domain.InstanceRelationDetail{
			InstanceRelation: r,
			Source:           refs[r.SourceInstanceID],
			Target:           refs[r.TargetInstanceID],
		}
	}

	return details, total, nil
}

func (s *relationService) Delete(ctx context.Context, tenantID int64, id int64) error {
	existing, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get relation: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrRelationNotFound
	}

	return s.repo.Delete(ctx, tenantID, id)
}

func (s *relationService) DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error {
	return s.repo.DeleteByInstanceID(ctx, tenantID, instanceID)
}

// DeriveRelations 从既有实例属性推导实例关系。
//
// 已确认的语义规则（2026-11-02 产品批准）：
//  1. 有向：source→target，沿用既有关系类型定义
//  2. 幂等：同一 (source, target, type) 已存在则跳过，不重复插入
//  3. 禁止自关联：source==target 丢弃
//  4. 允许环：不做环检测（真实拓扑可能存在环）
//  5. 手工优先：本方法**只补不存在的**，绝不覆盖/删除已有关系
//  6. 逐条结果：不做全量回滚，返回每条规则的统计
//
// 当前仅实现 ecs_disk 一条规则：实测 ecs 侧无磁盘引用字段，
// 而 disk 侧 `instance_id` 有 1312 条精确命中 ecs 的 asset_id，
// 708 台 ecs 全部可关联 —— 这是唯一在真实数据上成立的可推导关系。
func (s *relationService) DeriveRelations(
	ctx context.Context, tenantID int64, rule string, dryRun bool,
) (domain.DeriveResult, error) {
	result := domain.DeriveResult{DryRun: dryRun}

	if rule != "" && rule != "ecs_disk" {
		return result, fmt.Errorf("%w: 不支持的推导规则 %q（当前仅支持 ecs_disk）", errs.ErrInvalidRelation, rule)
	}

	// 读取两侧实例（均按租户过滤，避免跨租户泄漏）
	ecsList, err := s.instanceRepo.List(ctx, domain.InstanceFilter{
		TenantID: tenantID, ModelUID: "ecs", Limit: 100000,
	})
	if err != nil {
		return result, fmt.Errorf("failed to list ecs instances: %w", err)
	}
	diskList, err := s.instanceRepo.List(ctx, domain.InstanceFilter{
		TenantID: tenantID, ModelUID: "disk", Limit: 100000,
	})
	if err != nil {
		return result, fmt.Errorf("failed to list disk instances: %w", err)
	}

	candidates, stats := domain.DeriveEcsDisk(ecsList, diskList, "ecs_disk")

	if dryRun {
		// 仅统计已存在数量，不写入
		for _, c := range candidates {
			exists, err := s.repo.Exists(ctx, tenantID, c.SourceInstanceID, c.TargetInstanceID, c.RelationTypeUID)
			if err != nil {
				return result, fmt.Errorf("failed to check relation existence: %w", err)
			}
			if exists {
				stats.SkippedExisting++
			}
		}
		stats.Created = 0
		result.Rules = []domain.DeriveStats{stats}
		result.Samples = takeSamples(candidates, 5)
		return result, nil
	}

	// 实际写入：逐条判定，单条失败不影响其他（逐条结果，不回滚）
	for _, c := range candidates {
		exists, err := s.repo.Exists(ctx, tenantID, c.SourceInstanceID, c.TargetInstanceID, c.RelationTypeUID)
		if err != nil {
			return result, fmt.Errorf("failed to check relation existence: %w", err)
		}
		if exists {
			stats.SkippedExisting++
			continue
		}
		if _, err := s.repo.Create(ctx, domain.InstanceRelation{
			SourceInstanceID: c.SourceInstanceID,
			TargetInstanceID: c.TargetInstanceID,
			RelationTypeUID:  c.RelationTypeUID,
			TenantID:         tenantID,
		}); err != nil {
			s.logger.Warn("推导关系写入失败",
				elog.Int64("source", c.SourceInstanceID),
				elog.Int64("target", c.TargetInstanceID),
				elog.FieldErr(err))
			continue
		}
		stats.Created++
	}

	s.logger.Info("实例关系推导完成",
		elog.Int64("tenant_id", tenantID),
		elog.Int("candidate", stats.Candidate),
		elog.Int("created", stats.Created),
		elog.Int("skipped_existing", stats.SkippedExisting))

	result.Rules = []domain.DeriveStats{stats}
	result.Total = stats.Created
	result.Samples = takeSamples(candidates, 5)
	return result, nil
}

func takeSamples(c []domain.RelationCandidate, n int) []domain.RelationCandidate {
	if len(c) <= n {
		return c
	}
	return c[:n]
}
