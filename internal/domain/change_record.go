package domain

import (
	"encoding/json"
	"fmt"
)

// ChangeRecord 资产字段变更记录（与 e-cam-service 的 ecam_change_history 集合同构）。
//
// 背景（2026-11-02）：e-cam-service 已有审计查询面板（/api/v1/cam/audit/changes），
// 读取同一 mongo `ecam` 库的 `ecam_change_history` 集合；但该集合的写入链路
// （ChangeTracker.TrackChanges）**零调用方**，已停摆 24 天（最新记录 2026-10-09）。
//
// 本结构严格对齐 e-cam 的字段名与类型，使 e-cmdb-service 直接写入该集合后，
// 变更记录能被现有审计面板无缝消费，无需跨服务调用。
type ChangeRecord struct {
	ID           int64  `json:"id" bson:"id"`
	AssetID      string `json:"asset_id" bson:"asset_id"`
	AssetName    string `json:"asset_name" bson:"asset_name"`
	ModelUID     string `json:"model_uid" bson:"model_uid"`
	TenantID     int64  `json:"tenant_id" bson:"tenant_id"`
	AccountID    int64  `json:"account_id" bson:"account_id"`
	Provider     string `json:"provider" bson:"provider"`
	Region       string `json:"region" bson:"region"`
	FieldName    string `json:"field_name" bson:"field_name"`
	OldValue     string `json:"old_value" bson:"old_value"` // JSON 序列化
	NewValue     string `json:"new_value" bson:"new_value"` // JSON 序列化
	ChangeSource string `json:"change_source" bson:"change_source"`
	ChangeTaskID string `json:"change_task_id" bson:"change_task_id"`
	Ctime        int64  `json:"ctime" bson:"ctime"`
}

// ChangeMetadata 变更元数据（从实例上下文提取）。
type ChangeMetadata struct {
	AssetID      string
	AssetName    string
	ModelUID     string
	TenantID     int64
	AccountID    int64
	Provider     string
	Region       string
	ChangeSource string
	ChangeTaskID string
}

// 变更来源取值：与 e-cam 的 sync_task 区分，标识手工/CMDB 操作。
const (
	ChangeSourceManual = "manual"
	ChangeSourceSync   = "sync_task"
)

// ignoredChangeFields 与 e-cam 的 defaultIgnoreFields 对齐：这些字段高频变化
// 且不承载业务语义（同步时间戳等），记录它们只会产生噪音。
var ignoredChangeFields = map[string]bool{
	"sync_time":   true,
	"update_time": true,
	"utime":       true,
}

// FieldChange 一个字段的变更（old → new）。
type FieldChange struct {
	FieldName string
	OldValue  string // 空串表示「新增字段」
	NewValue  string // 空串表示「删除字段」
}

// DiffAttributes 对比新旧属性，产出字段级变更。
//
// 与 e-cam 的 ChangeTracker.TrackChanges 同构：
//   - 遍历 newAttrs：新增字段（old 不存在）记 OldValue=""
//   - 遍历 oldAttrs：删除字段（new 不存在）记 NewValue=""
//   - 值以 JSON 序列化后比较，相等则跳过
//   - 忽略 sync_time/update_time/utime
func DiffAttributes(oldAttrs, newAttrs map[string]interface{}) []FieldChange {
	var changes []FieldChange

	for key, newVal := range newAttrs {
		if ignoredChangeFields[key] {
			continue
		}
		oldVal, exists := oldAttrs[key]
		if !exists {
			changes = append(changes, FieldChange{FieldName: key, OldValue: "", NewValue: toChangeJSON(newVal)})
			continue
		}
		oldJSON := toChangeJSON(oldVal)
		newJSON := toChangeJSON(newVal)
		if oldJSON != newJSON {
			changes = append(changes, FieldChange{FieldName: key, OldValue: oldJSON, NewValue: newJSON})
		}
	}

	for key, oldVal := range oldAttrs {
		if ignoredChangeFields[key] {
			continue
		}
		if _, exists := newAttrs[key]; !exists {
			changes = append(changes, FieldChange{FieldName: key, OldValue: toChangeJSON(oldVal), NewValue: ""})
		}
	}
	return changes
}

// toChangeJSON 把值序列化为 JSON 字符串用于比较与存储。
// 与 e-cam 的 toJSON 完全一致：json.Marshal + fmt.Sprintf 兜底。
func toChangeJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
