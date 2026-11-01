package domain

import (
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// Instance 资产实例领域模型
type Instance struct {
	ID         int64                  // 业务ID
	ModelUID   string                 // 关联的模型UID (如 aliyun_ecs)
	AssetID    string                 // 云厂商资产ID (如 i-bp1234xxx)
	AssetName  string                 // 资产名称
	TenantID   int64                  // 租户ID
	AccountID  int64                  // 云账号ID
	Attributes map[string]interface{} // 动态属性
	CreateTime time.Time
	UpdateTime time.Time
}

// InstanceFilter 实例过滤条件
type InstanceFilter struct {
	ModelUID  string // 按模型过滤
	TenantID  int64  // 按租户过滤
	AccountID int64  // 按云账号过滤
	AssetName string // 按名称模糊搜索
	// AssetID 按资产 ID **精确匹配**。
	//
	// 背景（实测 2026-11-01）：实例列表接口此前**不读取** asset_id 查询参数，
	// 传该参数会被静默忽略并返回全量数据。调用方若以为筛选已生效
	// （例如用它定位单条记录后再做删改），会作用于远大于预期的集合。
	// 资产 ID 是唯一标识，精确匹配才是符合直觉的语义；模糊搜索仍走 AssetName
	// （其内部已同时对 asset_name 与 asset_id 做正则匹配）。
	AssetID    string
	Attributes map[string]interface{} // 按属性过滤
	// SortBy 排序字段（白名单见 dao 层）。空表示使用默认排序（ctime 倒序）。
	//
	// 背景（实测 2026-11-01）：实例列表此前**完全不支持排序** ——
	// 前端无列头排序 UI，后端 DAO 硬编码 `SetSort(ctime: -1)`，
	// 用户只能按创建时间倒序查看，无法按名称/状态/区域等排序。
	SortBy string
	// SortDesc true 为降序。仅在 SortBy 非空时生效。
	SortDesc bool
	Offset   int64
	Limit    int64
}

// Validate 验证实例数据
func (i *Instance) Validate() error {
	if i.ModelUID == "" {
		return errs.ErrInvalidModelUID
	}
	if i.AssetID == "" {
		return errs.ErrInvalidAssetID
	}
	if i.TenantID == 0 {
		return errs.ErrInvalidTenantID
	}
	return nil
}

// GetAttribute 获取指定属性值
func (i *Instance) GetAttribute(key string) (interface{}, bool) {
	if i.Attributes == nil {
		return nil, false
	}
	val, ok := i.Attributes[key]
	return val, ok
}

// SetAttribute 设置属性值
func (i *Instance) SetAttribute(key string, value interface{}) {
	if i.Attributes == nil {
		i.Attributes = make(map[string]interface{})
	}
	i.Attributes[key] = value
}

// GetStringAttribute 获取字符串类型属性
func (i *Instance) GetStringAttribute(key string) string {
	val, ok := i.GetAttribute(key)
	if !ok {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// GetIntAttribute 获取整数类型属性
func (i *Instance) GetIntAttribute(key string) int64 {
	val, ok := i.GetAttribute(key)
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}
