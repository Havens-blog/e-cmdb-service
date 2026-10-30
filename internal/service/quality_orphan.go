package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
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

// OrphanModelGroupView 某缺失模型下的孤儿实例数（API 返回用）。
type OrphanModelGroupView struct {
	ModelUID string `json:"model_uid"`
	Count    int64  `json:"count"`
}

// OrphanSourceResult 孤儿实例的来源分布。
type OrphanSourceResult struct {
	Total  int64                  `json:"total"`
	Groups []OrphanModelGroupView `json:"groups"`
}

// OrphanSources 按 model_uid 聚合孤儿实例来源。
//
// 解决的问题：数据质量页只展示「孤儿实例 12760」这个总数，
// 用户无法得知这些孤儿是**因为哪些模型缺失**造成的，也就不知道该补哪些模型。
// 实测（tenant 3，2026-10-31）：孤儿 12760 条来自 58 个未定义的 model_uid，
// 其中 volcengine_disk 1783、volcengine_snapshot 1491、aliyun_disk 1314、
// aliyun_snapshot 1205 等。拆开之后这个数字才是可执行的。
//
// 复用与 issuesOrphanByPushdown 相同的 knownModelUIDs 口径，
// 保证与孤儿总数、孤儿明细三者一致。
func (s *qualityService) OrphanSources(ctx context.Context, tenantID int64) (OrphanSourceResult, error) {
	models, err := s.modelRepo.List(ctx, domain.ModelFilter{})
	if err != nil {
		return OrphanSourceResult{}, fmt.Errorf("list models: %w", err)
	}
	known := make([]string, 0, len(models))
	for _, m := range models {
		if m.UID != "" {
			known = append(known, m.UID)
		}
	}

	rows, err := s.instanceRepo.GroupOrphanInstancesByModel(ctx, tenantID, known)
	if err != nil {
		return OrphanSourceResult{}, fmt.Errorf("group orphan instances: %w", err)
	}

	return buildOrphanSourceResult(rows), nil
}

// buildOrphanSourceResult 把 DAO 的分组结果组装成 API 返回结构（纯函数，便于单测）。
//
// 约定：
//   - Total 由各分组累加得出，不额外查一次 count —— 避免两次查询口径不一致
//     （例如并发写入导致 count 与分组结果不同步）。
//   - 保持 DAO 的降序，前端可直接按序展示 TopN。
//   - 空输入返回长度为 0 的切片（非 nil），前端 v-for 友好。
//   - 空 model_uid（model_uid 为 "" 或字段缺失）也计入 Total，
//     否则 Total 会小于 overview 的 orphan_count。
func buildOrphanSourceResult(rows []repository.OrphanModelGroup) OrphanSourceResult {
	out := OrphanSourceResult{Groups: make([]OrphanModelGroupView, 0, len(rows))}
	for _, r := range rows {
		out.Groups = append(out.Groups, OrphanModelGroupView{ModelUID: r.ModelUID, Count: r.Count})
		out.Total += r.Count
	}
	return out
}
