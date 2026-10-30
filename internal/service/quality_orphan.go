package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// issuesOrphanByPushdown 用服务端查询直接产出孤儿实例明细（不构造全量 issues 数组）。
//
// 背景：原 Issues 会先 loadAnalysis（把租户下全部实例拉进内存逐条判定，
// 实测约 5.4s）再按类型过滤分页。孤儿实例的判定条件可以完全下推到 Mongo
// （model_uid 不在模型表），因此可只取当前页的少量文档。
//
// 口径一致性：与 analyzeQuality 的孤儿判定保持一致 ——
//   - model_uid 不在模型表 → 孤儿
//   - model_uid 为空字符串 / 字段缺失 → 也判为孤儿
//
// 描述文案与字段填充均与 analyzeQuality 内一致。
func (s *qualityService) issuesOrphanByPushdown(
	ctx context.Context, tenantID int64, offset, limit int,
) (QualityIssuesResult, error) {
	models, err := s.modelRepo.List(ctx, domain.ModelFilter{})
	if err != nil {
		return QualityIssuesResult{}, fmt.Errorf("list models: %w", err)
	}
	known := make([]string, 0, len(models))
	for _, m := range models {
		if m.UID != "" {
			known = append(known, m.UID)
		}
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}

	rows, total, err := s.instanceRepo.ListOrphanInstances(ctx, tenantID, known, int64(offset), int64(limit))
	if err != nil {
		return QualityIssuesResult{}, fmt.Errorf("list orphan instances: %w", err)
	}

	items := make([]QualityIssue, len(rows))
	for i, r := range rows {
		items[i] = QualityIssue{
			Type:        QualityIssueTypeOrphan,
			Description: "实例所属模型不存在",
			InstanceID:  r.ID,
			AssetID:     r.AssetID,
			AssetName:   r.AssetName,
			ModelUID:    r.ModelUID,
		}
	}
	return QualityIssuesResult{Items: items, Total: int(total)}, nil
}
