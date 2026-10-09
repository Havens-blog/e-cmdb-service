package domain

import "time"

// InstanceRelation 实例关系领域模型
type InstanceRelation struct {
	ID               int64  // 业务ID
	SourceInstanceID int64  // 源实例ID
	TargetInstanceID int64  // 目标实例ID
	RelationTypeUID  string // 关系类型UID
	TenantID         int64  // 租户ID
	CreateTime       time.Time
}

// InstanceRelationFilter 关系过滤条件
type InstanceRelationFilter struct {
	SourceInstanceID int64
	TargetInstanceID int64
	RelationTypeUID  string
	TenantID         int64
	Offset           int64
	Limit            int64
}

// RelationType 关系类型定义
type RelationType struct {
	ID             int64  // 业务ID
	UID            string // 关系类型唯一标识
	Name           string // 关系名称
	SourceModelUID string // 源模型UID
	TargetModelUID string // 目标模型UID
	Direction      string // 方向: one_to_one, one_to_many, many_to_many
	Description    string // 描述
	CreateTime     time.Time
	UpdateTime     time.Time
}

// RelationTypeFilter 关系类型过滤条件
type RelationTypeFilter struct {
	SourceModelUID string
	TargetModelUID string
	Offset         int
	Limit          int
}
