package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// 数据质量问题类型（质量口径为「待产品确认」的合理默认实现）。
const (
	QualityIssueTypeOrphan           = "orphan_instance"
	QualityIssueTypeMissingField     = "missing_field"
	QualityIssueTypeDanglingRelation = "dangling_relation"
)

// QualityIssue 数据质量问题条目。
// 可选字段使用 omitempty：缺省时 JSON 中省略，但 JSON tag 名与前端契约保持一致。
type QualityIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	InstanceID  int64  `json:"instance_id,omitempty"`
	AssetID     string `json:"asset_id,omitempty"`
	AssetName   string `json:"asset_name,omitempty"`
	ModelUID    string `json:"model_uid,omitempty"`
	FieldUID    string `json:"field_uid,omitempty"`
	FieldName   string `json:"field_name,omitempty"`
	RelationID  int64  `json:"relation_id,omitempty"`
}

// QualityOverview 数据质量概览。
type QualityOverview struct {
	TotalInstances        int     `json:"total_instances"`
	OrphanCount           int     `json:"orphan_count"`
	MissingFieldCount     int     `json:"missing_field_count"`
	DanglingRelationCount int     `json:"dangling_relation_count"`
	HealthScore           float64 `json:"health_score"`
}

// QualityIssuesResult 数据质量问题列表。
type QualityIssuesResult struct {
	Items []QualityIssue `json:"items"`
	Total int            `json:"total"`
}

// qualityAnalysis 纯计算的中间结果，不直接对外序列化。
type qualityAnalysis struct {
	totalInstances        int
	orphanCount           int
	missingFieldCount     int
	danglingRelationCount int
	healthScore           float64
	issues                []QualityIssue

	// countsOnly 表示该结果只包含计数字段、successfully 不含 issues 明细。
	//
	// 由 overviewByCount（服务端计数快路径）写入缓存时置为 true。
	// 使用方必须检查：Issues 若读到 countsOnly 条目，应回退全量分析，
	// 否则会返回空列表（曾因此出现「概览 4447、明细 0 条」的口径矛盾）。
	countsOnly bool
}

// QualityService 数据质量服务接口。
type QualityService interface {
	Overview(ctx context.Context, tenantID int64) (QualityOverview, error)
	Issues(ctx context.Context, tenantID int64, typ string, offset, limit int) (QualityIssuesResult, error)
	// OrphanSources 按 model_uid 聚合孤儿实例来源（哪些模型缺失导致孤儿）
	OrphanSources(ctx context.Context, tenantID int64) (OrphanSourceResult, error)
}

type qualityService struct {
	instanceRepo repository.InstanceRepository
	modelRepo    repository.ModelRepository
	relationRepo repository.InstanceRelationRepository
	attrRepo     repository.AttributeRepository
	cache        *analysisCache
}

// analysisCacheTTL 质量分析结果缓存时长。
//
// 背景：loadAnalysis 会把租户下全部实例（实测 2 万余条）拉入内存逐条判定，
// 单次约 5 秒。而前端「数据质量」页一次加载会依次请求 overview 与 issues，
// 每次都要重算，导致每个请求都等 5 秒。
// 缓存后同一租户在 TTL 内只算一次，其余请求直接命中。
//
// 取值权衡：质量数据来源于定期同步，秒级实时性要求低；60 秒既能让页面
// 连续操作几乎免等待，又保证新增/变更数据最迟 1 分钟内可见。
const analysisCacheTTL = 60 * time.Second

// analysisCache 按租户缓存质量分析结果，并发安全。
type analysisCache struct {
	mu         sync.Mutex
	entries    map[int64]analysisEntry
	refreshing map[int64]bool
}

type analysisEntry struct {
	analysis qualityAnalysis
	expireAt time.Time
}

func newAnalysisCache() *analysisCache {
	return &analysisCache{entries: make(map[int64]analysisEntry), refreshing: make(map[int64]bool)}
}

// get 返回缓存结果及其是否已过期。
//
// 采用 stale-while-revalidate 语义：即使过期也把旧结果返回（stale=true），
// 由调用方决定是否立即返回旧值并异步刷新。这样用户不会因为 TTL 到期
// 而在请求路径上再等一次全量计算（实测约 5 秒）。
func (c *analysisCache) get(tenantID int64) (qualityAnalysis, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tenantID]
	if !ok {
		return qualityAnalysis{}, false, false
	}
	return e.analysis, true, time.Now().After(e.expireAt)
}

// tryBeginRefresh 标记某租户正在刷新，避免并发重复计算。返回是否获取到刷新权。
func (c *analysisCache) tryBeginRefresh(tenantID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshing[tenantID] {
		return false
	}
	c.refreshing[tenantID] = true
	return true
}

func (c *analysisCache) endRefresh(tenantID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.refreshing, tenantID)
}

func (c *analysisCache) put(tenantID int64, a qualityAnalysis) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[tenantID] = analysisEntry{analysis: a, expireAt: time.Now().Add(analysisCacheTTL)}
}

// NewQualityService 创建数据质量服务。
func NewQualityService(
	instanceRepo repository.InstanceRepository,
	modelRepo repository.ModelRepository,
	relationRepo repository.InstanceRelationRepository,
	attrRepo repository.AttributeRepository,
) QualityService {
	return &qualityService{
		instanceRepo: instanceRepo,
		modelRepo:    modelRepo,
		relationRepo: relationRepo,
		attrRepo:     attrRepo,
		cache:        newAnalysisCache(),
	}
}

// Overview 返回数据质量概览。
//
// 性能：概览只需要 4 个数字，不需要问题明细列表。
// 原实现走 loadAnalysis，会把租户下全部实例（实测 2.4 万条）拉进内存逐条判定，
// 单次约 5s。现改为：
//   - 缓存命中（未过期）  -> 直接返回（约 2ms）
//   - 缓存过期           -> 立刻返回旧值并异步刷新（用户不等）
//   - 无缓存             -> 服务端计数路径（实测约 500ms）
//
// 一致性保障：当存在实例关系时，dangling 无法只靠计数得出，
// 此时回退到原来的全量分析，确保口径与 Issues 完全一致。
func (s *qualityService) Overview(ctx context.Context, tenantID int64) (QualityOverview, error) {
	if s.cache != nil {
		if a, ok, stale := s.cache.get(tenantID); ok {
			if stale {
				s.refreshAsync(tenantID)
			}
			return QualityOverview{
				TotalInstances:        a.totalInstances,
				OrphanCount:           a.orphanCount,
				MissingFieldCount:     a.missingFieldCount,
				DanglingRelationCount: a.danglingRelationCount,
				HealthScore:           a.healthScore,
			}, nil
		}
	}

	if ov, ok, err := s.overviewByCount(ctx, tenantID); err == nil && ok {
		return ov, nil
	}

	// 回退：全量分析（口径与 Issues 一致）
	a, err := s.loadAnalysis(ctx, tenantID)
	if err != nil {
		return QualityOverview{}, err
	}
	return QualityOverview{
		TotalInstances:        a.totalInstances,
		OrphanCount:           a.orphanCount,
		MissingFieldCount:     a.missingFieldCount,
		DanglingRelationCount: a.danglingRelationCount,
		HealthScore:           a.healthScore,
	}, nil
}

// overviewByCount 用服务端聚合/计数计算概览，不把全量实例拉进内存。
//
// 返回 ok=false 表示当前数据形态不适合纯计数（例如存在实例关系，
// dangling 需要知道关系两端实例是否存在），调用方应回退全量分析。
func (s *qualityService) overviewByCount(ctx context.Context, tenantID int64) (QualityOverview, bool, error) {
	// 存在实例关系时不做纯计数，避免 dangling 口径与 Issues 不一致
	rels, err := s.relationRepo.List(ctx, domain.InstanceRelationFilter{TenantID: tenantID})
	if err != nil {
		return QualityOverview{}, false, err
	}
	if len(rels) > 0 {
		return QualityOverview{}, false, nil
	}

	// 模型名单（全局数据，量小）
	models, err := s.modelRepo.List(ctx, domain.ModelFilter{})
	if err != nil {
		return QualityOverview{}, false, err
	}
	modelSet := make(map[string]bool, len(models))
	for _, m := range models {
		modelSet[m.UID] = true
	}

	// 按 model_uid 分组统计实例数（一次聚合，实测约 50ms）
	byModel, err := s.instanceRepo.CountByModelUID(ctx, tenantID)
	if err != nil {
		return QualityOverview{}, false, err
	}

	total, orphan := 0, 0
	for uid, n := range byModel {
		total += int(n)
		if uid == "" || !modelSet[uid] {
			orphan += int(n)
		}
	}

	// 必填字段缺失：逐个 (model_uid, field_uid) 服务端计数后累加
	attrs, err := s.attrRepo.List(ctx, domain.AttributeFilter{})
	if err != nil {
		return QualityOverview{}, false, err
	}
	missing := 0
	for _, a := range attrs {
		if !a.Required || a.ModelUID == "" || a.FieldUID == "" {
			continue
		}
		// 模型已不存在的实例已计入 orphan，这里不重复计 missing
		if !modelSet[a.ModelUID] {
			continue
		}
		c, cerr := s.instanceRepo.CountMissingAttribute(ctx, tenantID, a.ModelUID, a.FieldUID)
		if cerr != nil {
			return QualityOverview{}, false, cerr
		}
		missing += int(c)
	}

	ov := QualityOverview{
		TotalInstances:        total,
		OrphanCount:           orphan,
		MissingFieldCount:     missing,
		DanglingRelationCount: 0, // 关系为空，悬空必为 0
		HealthScore:           computeHealthScore(total, orphan+missing),
	}

	if s.cache != nil {
		// 标记为「仅计数」：Issues 读到这种条目必须回退全量分析，
		// 否则会因 issues 为空而返回 0 条明细。
		s.cache.put(tenantID, qualityAnalysis{
			totalInstances:        ov.TotalInstances,
			orphanCount:           ov.OrphanCount,
			missingFieldCount:     ov.MissingFieldCount,
			danglingRelationCount: ov.DanglingRelationCount,
			healthScore:           ov.HealthScore,
			countsOnly:            true,
		})
	}
	return ov, true, nil
}

// Issues 返回指定类型的数据质量问题列表，支持分页。
//
// 取数策略：
//   - orphan_instance：判定条件可完全下推到 Mongo，直接按页查询
//     （实测首页约 70ms，相比全量分析约 5.4s）
//   - 其它类型：走缓存的分析结果；若缓存是 overviewByCount 写入的
//     「仅计数」条目（countsOnly=true），其 issues 为空，必须回退一次全量分析
//     （loadFresh 会把完整结果写回缓存，后续请求即可命中）
//
// 历史缺陷：曾直接复用仅计数条目，导致「概览 4447、明细 0 条」。
func (s *qualityService) Issues(ctx context.Context, tenantID int64, typ string, offset, limit int) (QualityIssuesResult, error) {
	if typ == QualityIssueTypeOrphan {
		return s.issuesOrphanByPushdown(ctx, tenantID, offset, limit)
	}
	if typ == QualityIssueTypeMissingField {
		return s.issuesMissingByPushdown(ctx, tenantID, offset, limit)
	}

	a, err := s.loadAnalysis(ctx, tenantID)
	if err != nil {
		return QualityIssuesResult{}, err
	}

	if needsFullAnalysisForIssues(a) {
		a, err = s.loadFresh(ctx, tenantID)
		if err != nil {
			return QualityIssuesResult{}, err
		}
	}

	return selectIssues(a.issues, typ, offset, limit), nil
}

// loadAnalysis 拉取数据后在内存中计算质量指标。模型/属性为全局数据，
// 实例/实例关系为租户作用域数据（按 tenantID 过滤）。
// loadAnalysis 带缓存的入口。本函数是全量扫描（实测约 5s），故：
//   - 无缓存     -> 同步计算（仅首次冷启动需要等待）
//   - 未过期     -> 直接返回缓存
//   - 已过期     -> 立即返回旧值，并异步刷新（用户不等待）
func (s *qualityService) loadAnalysis(ctx context.Context, tenantID int64) (qualityAnalysis, error) {
	if s.cache != nil {
		if a, ok, stale := s.cache.get(tenantID); ok {
			if stale {
				s.refreshAsync(tenantID)
			}
			return a, nil
		}
	}
	return s.loadFresh(ctx, tenantID)
}

// loadFresh 实际执行一次全量分析并写入缓存（不含缓存读取）。
func (s *qualityService) loadFresh(ctx context.Context, tenantID int64) (qualityAnalysis, error) {
	models, err := s.modelRepo.List(ctx, domain.ModelFilter{})
	if err != nil {
		return qualityAnalysis{}, fmt.Errorf("list models: %w", err)
	}

	attrs, err := s.attrRepo.List(ctx, domain.AttributeFilter{})
	if err != nil {
		return qualityAnalysis{}, fmt.Errorf("list attributes: %w", err)
	}

	instances, err := s.instanceRepo.List(ctx, domain.InstanceFilter{TenantID: tenantID})
	if err != nil {
		return qualityAnalysis{}, fmt.Errorf("list instances: %w", err)
	}

	relations, err := s.relationRepo.List(ctx, domain.InstanceRelationFilter{TenantID: tenantID})
	if err != nil {
		return qualityAnalysis{}, fmt.Errorf("list relations: %w", err)
	}

	result := analyzeQuality(models, attrs, instances, relations)
	if s.cache != nil {
		s.cache.put(tenantID, result)
	}
	return result, nil
}

// analyzeQuality 纯函数：输入已拉取的模型/属性/实例/关系列表，计算质量指标。
//
// 质量口径（待产品确认）：
//   - orphan：instance.ModelUID 不在任何 model.UID 中。
//   - missing_field：实例缺失其模型定义中 required=true 的属性字段；缺失
//     asset_id 或 model_uid 也视作 missing_field。模型无必填属性时不计。
//   - dangling_relation：实例关系的 source_instance_id 或 target_instance_id
//     在实例集合中不存在。
//
// 同一实例可能同时命中 orphan 与 missing_field（例如 model_uid 为空时），
// health_score 按问题总数计算，允许累加值超过实例总数并 clamp 到 0。
func analyzeQuality(models []domain.Model, attributes []domain.Attribute, instances []domain.Instance, relations []domain.InstanceRelation) qualityAnalysis {
	modelUIDs := make(map[string]struct{}, len(models))
	for _, m := range models {
		modelUIDs[m.UID] = struct{}{}
	}

	requiredByModel := make(map[string][]domain.Attribute)
	for _, attr := range attributes {
		if attr.Required {
			requiredByModel[attr.ModelUID] = append(requiredByModel[attr.ModelUID], attr)
		}
	}

	instanceIDs := make(map[int64]struct{}, len(instances))
	for _, inst := range instances {
		instanceIDs[inst.ID] = struct{}{}
	}

	var (
		issues   []QualityIssue
		orphan   int
		missing  int
		dangling int
	)

	for _, inst := range instances {
		_, modelExists := modelUIDs[inst.ModelUID]

		if !modelExists {
			orphan++
			issues = append(issues, QualityIssue{
				Type:        QualityIssueTypeOrphan,
				Description: "实例所属模型不存在",
				InstanceID:  inst.ID,
				AssetID:     inst.AssetID,
				AssetName:   inst.AssetName,
				ModelUID:    inst.ModelUID,
			})
		}

		if inst.AssetID == "" {
			missing++
			issues = append(issues, QualityIssue{
				Type:        QualityIssueTypeMissingField,
				Description: "缺失必填字段 asset_id",
				InstanceID:  inst.ID,
				AssetID:     inst.AssetID,
				AssetName:   inst.AssetName,
				ModelUID:    inst.ModelUID,
				FieldUID:    "asset_id",
				FieldName:   "asset_id",
			})
		}

		if inst.ModelUID == "" {
			missing++
			issues = append(issues, QualityIssue{
				Type:        QualityIssueTypeMissingField,
				Description: "缺失必填字段 model_uid",
				InstanceID:  inst.ID,
				AssetID:     inst.AssetID,
				AssetName:   inst.AssetName,
				FieldUID:    "model_uid",
				FieldName:   "model_uid",
			})
		}

		if modelExists {
			for _, attr := range requiredByModel[inst.ModelUID] {
				val, ok := inst.Attributes[attr.FieldUID]
				if !ok || val == nil {
					missing++
					issues = append(issues, QualityIssue{
						Type:        QualityIssueTypeMissingField,
						Description: fmt.Sprintf("缺失必填字段 %s", attr.FieldUID),
						InstanceID:  inst.ID,
						AssetID:     inst.AssetID,
						AssetName:   inst.AssetName,
						ModelUID:    inst.ModelUID,
						FieldUID:    attr.FieldUID,
						FieldName:   attr.FieldName,
					})
				}
			}
		}
	}

	for _, rel := range relations {
		_, srcOK := instanceIDs[rel.SourceInstanceID]
		_, dstOK := instanceIDs[rel.TargetInstanceID]
		if !srcOK || !dstOK {
			dangling++
			issues = append(issues, QualityIssue{
				Type:        QualityIssueTypeDanglingRelation,
				Description: fmt.Sprintf("关系引用的实例不存在（source=%d, target=%d）", rel.SourceInstanceID, rel.TargetInstanceID),
				RelationID:  rel.ID,
			})
		}
	}

	return qualityAnalysis{
		totalInstances:        len(instances),
		orphanCount:           orphan,
		missingFieldCount:     missing,
		danglingRelationCount: dangling,
		healthScore:           computeHealthScore(len(instances), orphan+missing+dangling),
		issues:                issues,
	}
}

// computeHealthScore 计算健康分：100 * (1 - issueCount / max(1, totalInstances))，
// clamp 到 [0,100] 后四舍五入到 1 位小数。
func computeHealthScore(totalInstances, issueCount int) float64 {
	denom := totalInstances
	if denom < 1 {
		denom = 1
	}

	score := 100.0 * (1.0 - float64(issueCount)/float64(denom))
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return math.Round(score*10) / 10
}

// selectIssues 按类型过滤并分页（纯函数，便于单测）。
func selectIssues(issues []QualityIssue, typ string, offset, limit int) QualityIssuesResult {
	filtered := make([]QualityIssue, 0)
	for _, issue := range issues {
		if issue.Type == typ {
			filtered = append(filtered, issue)
		}
	}

	total := len(filtered)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}

	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	return QualityIssuesResult{
		Items: filtered[start:end],
		Total: total,
	}
}

// refreshAsync 后台刷新指定租户的分析结果（单飞，避免并发重复计算）。
// 使用独立的 context：调用方请求可能已结束，不能让刷新随之取消。
func (s *qualityService) refreshAsync(tenantID int64) {
	if s.cache == nil || !s.cache.tryBeginRefresh(tenantID) {
		return
	}
	go func() {
		defer s.cache.endRefresh(tenantID)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := s.loadFresh(ctx, tenantID); err != nil {
			// 刷新失败保留旧值，等待下次请求再试；不向上抛错，避免干扰当前请求。
			return
		}
	}()
}

// needsFullAnalysisForIssues 判断给定分析结果能否直接用于明细查询。
//
// 概览快路径只写入计数字段（countsOnly=true），其 issues 为空，
// 直接用于明细会返回 0 条 —— 曾导致「概览 4447、明细 0 条」的口径矛盾。
// 抽出为纯函数便于单测覆盖。
func needsFullAnalysisForIssues(a qualityAnalysis) bool {
	return a.countsOnly
}
