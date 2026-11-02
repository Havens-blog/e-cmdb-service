package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// RelationService 实例关系服务接口
type RelationService interface {
	Create(ctx context.Context, relation domain.InstanceRelation) (int64, error)
	CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error)
	GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error)
	List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, int64, error)
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
	logger       *elog.Component
}

// NewRelationService 创建关系服务
func NewRelationService(repo repository.InstanceRelationRepository, instanceRepo repository.InstanceRepository) RelationService {
	return &relationService{
		repo:         repo,
		instanceRepo: instanceRepo,
		logger:       elog.DefaultLogger,
	}
}

func (s *relationService) Create(ctx context.Context, relation domain.InstanceRelation) (int64, error) {
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
