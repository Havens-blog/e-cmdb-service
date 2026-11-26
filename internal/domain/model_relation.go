package domain

import (
	"errors"
	"strings"
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

// Normalize 统一裁剪模型关系类型的文本字段。
//
// 为什么不放在 service：Validate 是导出的领域校验，任何调用方（含将来的批处理、
// 迁移脚本）都可能直接调用它；若裁剪只写在 service.Create/Update 里，
// 直接调 Validate 的路径就会把 " " 当成合法名称落库。与 Instance.Normalize
// 一样把「文本字段如何取值」的规则收在领域层，调用方无需各自记得先裁剪。
//
// 与 Instance 的差异（易被误读为等价）：Instance.Normalize 由 service 显式调用，
// 而这里的裁剪切在 Validate 内部，所以 **Validate 是唯一入口**——任何新增的
// 「不校验只落库」路径（如批量导入、数据修复脚本直接写仓储）都必须自己先调
// Normalize，否则会绕过裁剪。
//
// SourceToTarget / TargetToSource 会作为拓扑边的双向标签展示，
// Description 会在详情里展示，三者与 Name 同属可选描述性文本，
// 因此用同一套规则（首尾空白裁掉），避免出现"看得见但全是空格"的标签。
func (m *ModelRelationType) Normalize() {
	m.UID = strings.TrimSpace(m.UID)
	m.Name = strings.TrimSpace(m.Name)
	m.SourceModelUID = strings.TrimSpace(m.SourceModelUID)
	m.TargetModelUID = strings.TrimSpace(m.TargetModelUID)
	m.SourceToTarget = strings.TrimSpace(m.SourceToTarget)
	m.TargetToSource = strings.TrimSpace(m.TargetToSource)
	m.Description = strings.TrimSpace(m.Description)
}

// Validate 验证模型关系类型。
//
// 先 Normalize 再判空：HTTP 层的 binding:"required" 只拒空串，" " 能通过；
// 若不在校验前裁剪，空白名称会被判为合法并落库，列表与拓扑边标签全部空白。
// 裁剪放在这里（而非仅调用方），保证直接调用 Validate 的路径也不会漏。
func (m *ModelRelationType) Validate() error {
	m.Normalize()
	if m.UID == "" {
		return ErrInvalidRelationTypeUID
	}
	if m.Name == "" {
		return ErrInvalidRelationName
	}
	if m.SourceModelUID == "" || m.TargetModelUID == "" {
		return ErrInvalidRelationModel
	}
	return nil
}

// ErrInvalidRelationTypeUID 无效的关系类型UID
var ErrInvalidRelationTypeUID = errors.New("relation type uid cannot be empty")

// ErrInvalidRelationName 无效的关系类型名称
var ErrInvalidRelationName = errors.New("relation type name cannot be empty")

// ErrInvalidRelationModel 无效的关系模型
var ErrInvalidRelationModel = errors.New("source and target model uid cannot be empty")
