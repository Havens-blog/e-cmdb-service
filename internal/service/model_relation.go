package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotomicro/ego/core/elog"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
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
	// modelRepo 只用于 Create/Update 时校验 source/target 模型确实存在。
	modelRepo repository.ModelRepository
	logger    *elog.Component
}

// NewModelRelationTypeService 创建模型关系类型服务。
//
// instRelRepo 用于删除保护：仍被资产关系引用的类型不允许删除，
// 否则会留下 relation_type_uid 指向已删类型的孤儿关系（实测 2026-11-19：
// 删除在用类型返回 code=0，关系仍在，但前端已拿不到中文类型名）；传 nil 跳过。
// modelRepo 用于 Create/Update 时校验声明的源/目标模型存在，防止再造悬空关系类型；
// 传 nil 时跳过该校验（保留给尚未装配模型仓储的测试与旧调用点）。
func NewModelRelationTypeService(repo repository.ModelRelationTypeRepository, instRelRepo repository.InstanceRelationRepository, modelRepo repository.ModelRepository) ModelRelationTypeService {
	return &modelRelationTypeService{
		repo:        repo,
		instRelRepo: instRelRepo,
		modelRepo:   modelRepo,
		logger:      elog.DefaultLogger,
	}
}

func (s *modelRelationTypeService) Create(ctx context.Context, rel domain.ModelRelationType) (int64, error) {
	// 裁剪与判空都在 domain.Validate 内完成（Validate 先 Normalize）：binding:"required" 只拒空串，
	// 单个空格能进到这里，存成 " " 的 uid 会让关系类型在列表里「看得见、选不着」。

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
	if err := s.ensureSourceModelExists(ctx, sourceModelUID); err != nil {
		return err
	}
	return s.ensureTargetModelExists(ctx, targetModelUID)
}

func (s *modelRelationTypeService) ensureSourceModelExists(ctx context.Context, sourceModelUID string) error {
	if s.modelRepo == nil {
		return nil
	}
	model, err := s.modelRepo.GetByUID(ctx, sourceModelUID)
	if err != nil {
		// 不把 uid 拼进错误：这条最终会被 handler 兜成 5xx 用户可见文案，
		// 原始驱动错误已经由 %w 保留给日志，没必要把标识泄漏到响应里。
		return fmt.Errorf("failed to load relation source model: %w", err)
	}
	if model.ID == 0 {
		// 只回哨兵：这条最终可能被 handler 兜成 5xx 并把 err.Error() 原样暴露，
		// 标识信息留给日志，不进响应体。
		s.logger.Warn("关系类型引用的源模型不存在",
			elog.String("source_model_uid", sourceModelUID))
		return errs.ErrRelationSourceModelNotFound
	}
	return nil
}

func (s *modelRelationTypeService) ensureTargetModelExists(ctx context.Context, targetModelUID string) error {
	if s.modelRepo == nil {
		return nil
	}
	model, err := s.modelRepo.GetByUID(ctx, targetModelUID)
	if err != nil {
		return fmt.Errorf("failed to load relation target model: %w", err)
	}
	if model.ID == 0 {
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

// Update 更新模型关系类型。
//
// 契约：入参是「合并后的完整对象」，不是补丁。
// HTTP 层（UpdateModelRelation）会先 GetByUID 取旧值，只把用户显式提供的字段覆盖上去，
// 所以 Name 为空只可能来自两类历史数据或程序化调用：
//   - 早先落库的空白名称记录（本 PR 修复前 Create 不裁剪，存过 " "）；
//   - 直接调用 service 的代码。
//
// 这类记录要改名必须在该次 PUT 里带上 Name —— 只改 description 会被 Validate 拒绝并返回
// 「关系类型名称不能为空」，这是刻意选择：静默保留空白名称会让问题数据永远无法被发现。
//
// 裁剪与校验都在 domain.Validate 内完成（Validate 先 Normalize），
// 这里不再重复 TrimSpace，避免两层各写一份日后漂移。
func (s *modelRelationTypeService) Update(ctx context.Context, rel domain.ModelRelationType) error {
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

	if err := s.ensureChangedModelsExist(ctx, rel); err != nil {
		return err
	}

	return s.repo.Update(ctx, rel)
}

// ensureChangedModelsExist 只校验本次真正改动的那一侧模型引用。
//
// 与 Create 的区别：Create 校验整条声明（用户正在写这两个值），Update 面对的是**已存在**的行，
// 而 UpdateModelRelationReq 至今不能改 SourceModelUID/TargetModelUID。若这里无条件地校验两侧，
// 存量脏数据（实测但未修：ecs_belong_to_vpc → cloud_ecs，cloud_ecs 模型已不存在）
// 就连「只改描述」都会 400011 失败，而请求里根本没有可指向合法模型的字段——
// 等于把历史债务变成了永久不可编辑，比留下脏数据更糟。
//
// 因此口径改为「只校验本次写入的值」：某一侧与库中一致即视为未改动、跳过校验；
// 某侧被改动（现在只能来自程序化调用，将来开放编辑后来自 HTTP）则必须存在。
// 读库失败时保守回落到「两侧都校验」——宁可拦住也不放行未知状态。
func (s *modelRelationTypeService) ensureChangedModelsExist(ctx context.Context, rel domain.ModelRelationType) error {
	if s.modelRepo == nil {
		return nil
	}

	stored, err := s.repo.GetByUID(ctx, rel.UID)
	if err != nil {
		// 读不到库 ≠ 行不存在：把仓储故障当成「两侧都校验」会掩盖真实错误，
		// 甚至在两侧都合法时把一次读失败变成「更新成功」。原样上抛。
		return fmt.Errorf("failed to load relation type %q: %w", rel.UID, err)
	}
	if stored.ID == 0 {
		return s.ensureModelsExist(ctx, rel.SourceModelUID, rel.TargetModelUID)
	}

	// 库里的旧值可能带空白（早于 domain.Validate 裁剪就已存在的行）：不先裁剪，
	// 一条 " ecs " 的行会被判成「本次改动了源模型」，正好让本函数想放行的
	// 「存量脏数据只改描述」重新被拦。
	storedSource := strings.TrimSpace(stored.SourceModelUID)
	storedTarget := strings.TrimSpace(stored.TargetModelUID)

	if rel.SourceModelUID != storedSource {
		if err := s.ensureSourceModelExists(ctx, rel.SourceModelUID); err != nil {
			return err
		}
	}
	if rel.TargetModelUID != storedTarget {
		if err := s.ensureTargetModelExists(ctx, rel.TargetModelUID); err != nil {
			return err
		}
	}
	return nil
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
