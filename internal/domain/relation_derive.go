package domain

import "strings"

// RelationCandidate 一条待建立的实例关系（推导产物）
type RelationCandidate struct {
	SourceInstanceID int64
	TargetInstanceID int64
	RelationTypeUID  string
	// SourceAssetID / TargetAssetID 仅用于回显与排查，不入库
	SourceAssetID string
	TargetAssetID string
}

// DeriveStats 单个推导规则的统计
type DeriveStats struct {
	// Rule 规则标识，如 ecs_disk
	Rule string `json:"rule"`
	// Scanned 扫描的候选侧实例数（本例为 disk 数）
	Scanned int `json:"scanned"`
	// Matched 找到对端实例的数量
	Matched int `json:"matched"`
	// Candidate 去重后的候选关系数
	Candidate int `json:"candidate"`
	// Created 实际新建数（dry-run 时为 0）
	Created int `json:"created"`
	// SkippedExisting 已存在而跳过（幂等）
	SkippedExisting int `json:"skipped_existing"`
	// SkippedSelf 自关联被拒（规则：禁止）
	SkippedSelf int `json:"skipped_self"`
	// Unresolved 引用无法对上任何实例（多为同步范围未覆盖的账号/区域）
	Unresolved int `json:"unresolved"`
}

// DeriveResult 推导汇总
type DeriveResult struct {
	DryRun bool          `json:"dry_run"`
	Rules  []DeriveStats `json:"rules"`
	Total  int           `json:"total_created"`
	// Samples 少量候选样本，便于人工核对（dry-run 时尤其有用）
	Samples []RelationCandidate `json:"samples"`
}

// DeriveEcsDisk 从「云盘子对象侧」推导 ecs→disk 关系。
//
// 设计依据（实测 2026-11-02）：
//   - ecs 侧**没有**磁盘引用字段（system_disk_id 为空、data_disks 为空）→ 无法从 ecs 推导
//   - disk 侧有 `instance_id`，3415/3427 条有值，其中 **1312 条精确命中 ecs 的 asset_id**
//   - 命中全部来自云账号 1（ecs 也只采了账号 1），708 台 ecs **100% 都能关联到磁盘**
//
// 因此采用「从子对象反向推导」：disk.instance_id → ecs.asset_id。
// 关系方向按既有类型定义 `ecs_disk`（ecs → disk, one_to_many）。
//
// 幂等：同一 (source, target, type) 只产生一条候选；是否已存在由调用方用 Exists 判定。
// 自关联：source == target 一律丢弃（已确认规则：禁止自关联）。
func DeriveEcsDisk(
	ecsInstances []Instance,
	diskInstances []Instance,
	relationTypeUID string,
) ([]RelationCandidate, DeriveStats) {
	stats := DeriveStats{Rule: relationTypeUID, Scanned: len(diskInstances)}

	// ecs: asset_id -> instance id
	ecsByAsset := make(map[string]int64, len(ecsInstances))
	for _, e := range ecsInstances {
		asset := strings.TrimSpace(e.AssetID)
		if asset == "" {
			continue
		}
		ecsByAsset[asset] = e.ID
	}

	seen := make(map[string]bool, len(diskInstances))
	candidates := make([]RelationCandidate, 0, len(diskInstances))

	for _, d := range diskInstances {
		ref := ""
		if d.Attributes != nil {
			if v, ok := d.Attributes["instance_id"]; ok && v != nil {
				ref = strings.TrimSpace(scalarToString(v))
			}
		}
		if ref == "" {
			continue
		}

		srcID, ok := ecsByAsset[ref]
		if !ok {
			// 引用存在但目标实例未被采集（账号/区域未覆盖）
			stats.Unresolved++
			continue
		}
		stats.Matched++

		if srcID == d.ID {
			// 理论上 ecs 与 disk 不同模型不会相等，仍按规则显式拒绝
			stats.SkippedSelf++
			continue
		}

		key := ref + "|" + strings.TrimSpace(d.AssetID)
		if seen[key] {
			continue
		}
		seen[key] = true

		candidates = append(candidates, RelationCandidate{
			SourceInstanceID: srcID,
			TargetInstanceID: d.ID,
			RelationTypeUID:  relationTypeUID,
			SourceAssetID:    ref,
			TargetAssetID:    d.AssetID,
		})
	}

	stats.Candidate = len(candidates)
	return candidates, stats
}
