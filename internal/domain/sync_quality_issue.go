package domain

import "time"

// SyncQualityIssue 同步写入时发现的实例字段质量问题（记录但不阻断）。
//
// 背景（2026-11-02，产品方案 B）：同步链路（upsert-batch）是最大的数据写入入口，
// 但此前完全不做模型字段校验，导致脏数据持续进入（实测 4132 条实例缺必填字段）。
// 方案 B 要求：采集不中断，同时把问题「记录」下来、形成可查询的清单。
//
// 去重键 = (tenant_id, model_uid, asset_id, issue_type, field_uid)：
// 同一实例同一字段的同一类问题只保留一条，避免每次同步都无界增长。
type SyncQualityIssue struct {
	ID        int64  `json:"id" bson:"id"`
	TenantID  int64  `json:"tenant_id" bson:"tenant_id"`
	ModelUID  string `json:"model_uid" bson:"model_uid"`
	AssetID   string `json:"asset_id" bson:"asset_id"`
	AssetName string `json:"asset_name" bson:"asset_name"`
	// IssueType 与字段校验问题类型对齐：required / enum / type
	IssueType string `json:"issue_type" bson:"issue_type"`
	FieldUID  string `json:"field_uid" bson:"field_uid"`
	Message   string `json:"message" bson:"message"`
	// LastSeen 最近一次发现该问题的时间（UnixMilli）
	LastSeen int64 `json:"last_seen" bson:"last_seen"`
	// CreateTime 首次发现时间
	CreateTime time.Time `json:"create_time" bson:"ctime"`
}

// SyncQualityIssueFilter 同步质量问题查询过滤
type SyncQualityIssueFilter struct {
	TenantID  int64
	ModelUID  string
	IssueType string
	AssetID   string
	Offset    int64
	Limit     int64
}
