package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// missingFieldGroup 某个 (模型, 字段) 组合的缺失实例数。
type missingFieldGroup struct {
	ModelUID  string
	FieldUID  string
	FieldName string
	Count     int64
}

// issuesMissingByPushdown 用服务端查询产出「缺失必填字段」明细（不构造全量 issues 数组）。
//
// 口径说明（与 analyzeQuality 保持一致）：
//   - 该问题类型是**逐字段**语义：同一实例缺 2 个必填字段会出现 2 条明细，
//     因此 total = 各 (模型,字段) 组合的缺失数之和，而不是「缺失实例数」。
//   - 只统计模型仍存在的组合（模型已不存在的实例计入 orphan_instance）。
//
// 分页方式：先按 (model_uid, field_uid) 字典序遍历组合并取各自计数，
// 用累计偏移定位当前页落在哪些组合上，再只对这些组合按 skip/limit 取实例。
// 这样不需要把全部实例拉进内存（原实现实测约 5.4s）。
//
// 注意：排序口径与原实现不同 —— 原实现按「实例遍历顺序 → 该实例的必填字段顺序」，
// 本实现按「模型 → 字段 → 实例 id」。总数一致，但同一页展示的条目集合可能不同。
// 该变化是有意的：按字段分组更便于用户批量定位同一字段的缺失情况。
func (s *qualityService) issuesMissingByPushdown(
	ctx context.Context, tenantID int64, offset, limit int,
) (QualityIssuesResult, error) {
	models, err := s.modelRepo.List(ctx, domain.ModelFilter{})
	if err != nil {
		return QualityIssuesResult{}, fmt.Errorf("list models: %w", err)
	}
	modelSet := make(map[string]bool, len(models))
	for _, m := range models {
		modelSet[m.UID] = true
	}

	attrs, err := s.attrRepo.List(ctx, domain.AttributeFilter{})
	if err != nil {
		return QualityIssuesResult{}, fmt.Errorf("list attributes: %w", err)
	}

	// 收集「模型存在 + 必填 + 有点位」的字段组合
	type key struct{ model, field, name string }
	seen := make(map[key]bool)
	keys := make([]key, 0, len(attrs))
	for _, a := range attrs {
		if !a.Required || a.ModelUID == "" || a.FieldUID == "" || !modelSet[a.ModelUID] {
			continue
		}
		k := key{a.ModelUID, a.FieldUID, a.FieldName}
		if seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}

	// 字典序保证顺序稳定（分页一致性）
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].model != keys[j].model {
			return keys[i].model < keys[j].model
		}
		return keys[i].field < keys[j].field
	})

	groups := make([]missingFieldGroup, 0, len(keys))
	var total int64
	for _, k := range keys {
		c, cerr := s.instanceRepo.CountMissingFieldGroup(ctx, tenantID, k.model, k.field)
		if cerr != nil {
			return QualityIssuesResult{}, fmt.Errorf("count missing %s.%s: %w", k.model, k.field, cerr)
		}
		if c <= 0 {
			continue
		}
		groups = append(groups, missingFieldGroup{k.model, k.field, k.name, c})
		total += c
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}

	items := make([]QualityIssue, 0, limit)
	var cursor int64 // 已跳过的条目数
	remain := limit
	for _, g := range groups {
		if int64(len(items)) >= int64(limit) {
			break
		}
		// 当前页尚未进入该组
		if cursor+g.Count <= int64(offset) {
			cursor += g.Count
			continue
		}
		var skip int64
		if cursor < int64(offset) {
			skip = int64(offset) - cursor
		}
		rows, lerr := s.instanceRepo.ListMissingFieldGroup(ctx, tenantID, g.ModelUID, g.FieldUID, skip, int64(remain))
		if lerr != nil {
			return QualityIssuesResult{}, fmt.Errorf("list missing %s.%s: %w", g.ModelUID, g.FieldUID, lerr)
		}
		for _, r := range rows {
			items = append(items, QualityIssue{
				Type:        QualityIssueTypeMissingField,
				Description: "缺失必填字段 " + g.FieldUID,
				InstanceID:  r.ID,
				AssetID:     r.AssetID,
				AssetName:   r.AssetName,
				ModelUID:    r.ModelUID,
				FieldUID:    g.FieldUID,
				FieldName:   g.FieldName,
			})
		}
		remain -= len(rows)
		cursor += g.Count
	}

	return QualityIssuesResult{Items: items, Total: int(total)}, nil
}
