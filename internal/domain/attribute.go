package domain

import (
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// Attribute 模型属性/字段定义
type Attribute struct {
	ID          int64       // 业务ID
	FieldUID    string      // 字段唯一标识
	FieldName   string      // 字段名称
	FieldType   string      // 字段类型: string, int, float, bool, enum, datetime, array, json, link
	ModelUID    string      // 所属模型UID
	GroupID     int64       // 字段分组ID
	DisplayName string      // 显示名称
	Display     bool        // 是否显示
	Index       int         // 排序索引
	Required    bool        // 是否必填
	Editable    bool        // 是否可编辑
	Searchable  bool        // 是否可搜索
	Unique      bool        // 是否唯一
	Secure      bool        // 是否敏感字段
	Link        bool        // 是否关联字段
	LinkModel   string      // 关联的模型UID
	Option      interface{} // 选项值(用于enum类型，可以是数组或JSON字符串)
	Default     string      // 默认值
	Placeholder string      // 占位提示
	Description string      // 字段描述
	CreateTime  time.Time   // 创建时间
	UpdateTime  time.Time   // 更新时间
}

// AttributeFilter 属性过滤条件
type AttributeFilter struct {
	ModelUID   string
	GroupID    int64
	FieldType  string
	Display    *bool
	Required   *bool
	Searchable *bool
	Offset     int
	Limit      int
}

// Validate 验证属性数据
func (a *Attribute) Validate() error {
	if a.FieldUID == "" {
		return errs.ErrInvalidAttributeUID
	}
	if a.FieldName == "" {
		return errs.ErrInvalidAttributeName
	}
	if a.ModelUID == "" {
		return errs.ErrInvalidModelUID
	}
	if !IsValidFieldType(a.FieldType) {
		return errs.ErrInvalidAttributeType
	}
	return nil
}

// 字段类型常量
const (
	FIELD_TYPE_STRING   = "string"
	FIELD_TYPE_INT      = "int"
	FIELD_TYPE_FLOAT    = "float"
	FIELD_TYPE_BOOL     = "bool"
	FIELD_TYPE_ENUM     = "enum"
	FIELD_TYPE_DATETIME = "datetime"
	FIELD_TYPE_DATE     = "date"
	FIELD_TYPE_ARRAY    = "array"
	FIELD_TYPE_JSON     = "json"
	FIELD_TYPE_LINK     = "link"
	FIELD_TYPE_TEXT     = "text" // 长文本
)

// IsValidFieldType 检查字段类型是否有效
func IsValidFieldType(fieldType string) bool {
	validTypes := map[string]bool{
		FIELD_TYPE_STRING:   true,
		FIELD_TYPE_INT:      true,
		FIELD_TYPE_FLOAT:    true,
		FIELD_TYPE_BOOL:     true,
		FIELD_TYPE_ENUM:     true,
		FIELD_TYPE_DATETIME: true,
		FIELD_TYPE_DATE:     true,
		FIELD_TYPE_ARRAY:    true,
		FIELD_TYPE_JSON:     true,
		FIELD_TYPE_LINK:     true,
		FIELD_TYPE_TEXT:     true,
	}
	return validTypes[fieldType]
}

// GetFieldTypes 获取所有支持的字段类型
func GetFieldTypes() []map[string]string {
	return []map[string]string{
		{"value": FIELD_TYPE_STRING, "label": "短文本"},
		{"value": FIELD_TYPE_TEXT, "label": "长文本"},
		{"value": FIELD_TYPE_INT, "label": "整数"},
		{"value": FIELD_TYPE_FLOAT, "label": "浮点数"},
		{"value": FIELD_TYPE_BOOL, "label": "布尔值"},
		{"value": FIELD_TYPE_ENUM, "label": "枚举"},
		{"value": FIELD_TYPE_DATETIME, "label": "日期时间"},
		{"value": FIELD_TYPE_DATE, "label": "日期"},
		{"value": FIELD_TYPE_ARRAY, "label": "数组"},
		{"value": FIELD_TYPE_JSON, "label": "JSON"},
		{"value": FIELD_TYPE_LINK, "label": "关联模型"},
	}
}

// AttributeGroup 属性分组
type AttributeGroup struct {
	ID          int64     // 业务ID
	UID         string    // 分组唯一标识
	Name        string    // 分组名称
	ModelUID    string    // 所属模型UID
	Index       int       // 排序索引
	IsBuiltin   bool      // 是否内置分组
	Description string    // 描述
	CreateTime  time.Time // 创建时间
	UpdateTime  time.Time // 更新时间
}

// AttributeGroupFilter 属性分组过滤条件
type AttributeGroupFilter struct {
	ModelUID  string
	IsBuiltin *bool
	Offset    int
	Limit     int
}

// AttributeGroupWithAttrs 带属性列表的分组
type AttributeGroupWithAttrs struct {
	AttributeGroup
	Attributes []Attribute `json:"attributes"`
}

// 预置属性分组UID
const (
	ATTR_GROUP_BASIC    = "basic"    // 基本信息
	ATTR_GROUP_NETWORK  = "network"  // 网络信息
	ATTR_GROUP_RESOURCE = "resource" // 资源信息
	ATTR_GROUP_TIME     = "time"     // 时间信息
	ATTR_GROUP_CUSTOM   = "custom"   // 自定义
)

// GetBuiltinAttributeGroups 获取预置属性分组
func GetBuiltinAttributeGroups(modelUID string) []AttributeGroup {
	return []AttributeGroup{
		{UID: ATTR_GROUP_BASIC, Name: "基本信息", ModelUID: modelUID, Index: 1, IsBuiltin: true},
		{UID: ATTR_GROUP_NETWORK, Name: "网络信息", ModelUID: modelUID, Index: 2, IsBuiltin: true},
		{UID: ATTR_GROUP_RESOURCE, Name: "资源信息", ModelUID: modelUID, Index: 3, IsBuiltin: true},
		{UID: ATTR_GROUP_TIME, Name: "时间信息", ModelUID: modelUID, Index: 4, IsBuiltin: true},
		{UID: ATTR_GROUP_CUSTOM, Name: "自定义字段", ModelUID: modelUID, Index: 100, IsBuiltin: true},
	}
}
