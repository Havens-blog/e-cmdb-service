package domain

import (
	"errors"
	"time"
)

// 关系方向常量
const (
	RelationDirectionOneToOne   = "one_to_one"
	RelationDirectionOneToMany  = "one_to_many"
	RelationDirectionManyToMany = "many_to_many"
)

// 关系类型常量
const (
	RelationTypeBelongsTo = "belongs_to" // 从属关系
	RelationTypeContains  = "contains"   // 包含关系
	RelationTypeBindTo    = "bindto"     // 绑定关系
	RelationTypeConnects  = "connects"   // 连接关系
	RelationTypeDependsOn = "depends_on" // 依赖关系
)

// ModelRelationType 模型关系类型定义
// 定义两个模型之间可以建立什么样的关系
type ModelRelationType struct {
	ID             int64  // 业务ID
	UID            string // 关系类型唯一标识，如 ecs_bindto_eip
	Name           string // 关系名称，如 "ECS绑定EIP"
	SourceModelUID string // 源模型UID
	TargetModelUID string // 目标模型UID
	RelationType   string // 关系类型: belongs_to, contains, bindto, connects, depends_on
	Direction      string // 方向: one_to_one, one_to_many, many_to_many
	SourceToTarget string // 源到目标的描述，如 "绑定"
	TargetToSource string // 目标到源的描述，如 "被绑定"
	Description    string // 描述
	CreateTime     time.Time
	UpdateTime     time.Time
}

// ModelRelationTypeFilter 模型关系类型过滤条件
type ModelRelationTypeFilter struct {
	SourceModelUID string
	TargetModelUID string
	RelationType   string
	Offset         int
	Limit          int
}

// Validate 验证模型关系类型
func (m *ModelRelationType) Validate() error {
	if m.UID == "" {
		return ErrInvalidRelationTypeUID
	}
	if m.SourceModelUID == "" || m.TargetModelUID == "" {
		return ErrInvalidRelationModel
	}
	return nil
}

// ErrInvalidRelationTypeUID 无效的关系类型UID
var ErrInvalidRelationTypeUID = errors.New("relation type uid cannot be empty")

// ErrInvalidRelationModel 无效的关系模型
var ErrInvalidRelationModel = errors.New("source and target model uid cannot be empty")
