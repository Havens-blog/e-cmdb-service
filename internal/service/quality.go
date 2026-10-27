package service

import (
	"context"
	"fmt"
	"math"

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
}

// QualityService 数据质量服务接口。
type QualityService interface {
	Overview(ctx context.Context, tenantID int64) (QualityOverview, error)
	Issues(ctx context.Context, tenantID int64, typ string, offset, limit int) (QualityIssuesResult, error)
}

type qualityService struct {
	instanceRepo repository.InstanceRepository
	modelRepo    repository.ModelRepository
	relationRepo repository.InstanceRelationRepository
	attrRepo     repository.AttributeRepository
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
	}
}

// Overview 返回数据质量概览。
func (s *qualityService) Overview(ctx context.Context, tenantID int64) (QualityOverview, error) {
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

// Issues 返回指定类型的数据质量问题列表，支持分页。
func (s *qualityService) Issues(ctx context.Context, tenantID int64, typ string, offset, limit int) (QualityIssuesResult, error) {
	a, err := s.loadAnalysis(ctx, tenantID)
	if err != nil {
		return QualityIssuesResult{}, err
	}

	return selectIssues(a.issues, typ, offset, limit), nil
}

// loadAnalysis 拉取数据后在内存中计算质量指标。模型/属性为全局数据，
// 实例/实例关系为租户作用域数据（按 tenantID 过滤）。
func (s *qualityService) loadAnalysis(ctx context.Context, tenantID int64) (qualityAnalysis, error) {
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

	return analyzeQuality(models, attrs, instances, relations), nil
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
