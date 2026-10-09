package domain

import "time"

// ModelGroup 模型分组（模型分类）
// 参考蓝鲸CMDB的bk_classification设计
type ModelGroup struct {
	ID          int64     `json:"id"`
	UID         string    `json:"uid"`         // 分组唯一标识，如 host, network, cloud
	Name        string    `json:"name"`        // 分组名称，如 主机管理, 网络设备, 云资源
	Icon        string    `json:"icon"`        // 分组图标
	SortOrder   int       `json:"sort_order"`  // 排序顺序
	IsBuiltin   bool      `json:"is_builtin"`  // 是否内置分组
	Description string    `json:"description"` // 描述
	CreateTime  time.Time `json:"create_time"`
	UpdateTime  time.Time `json:"update_time"`
}

// ModelGroupFilter 模型分组查询条件
type ModelGroupFilter struct {
	UID       string
	IsBuiltin *bool
	Offset    int
	Limit     int
}

// ModelGroupWithModels 带模型列表的分组
type ModelGroupWithModels struct {
	ModelGroup
	Models []Model `json:"models"`
}

// 预置分组UID常量
const (
	MODEL_GROUP_HOST       = "host"       // 主机管理
	MODEL_GROUP_NETWORK    = "network"    // 网络设备
	MODEL_GROUP_CLOUD      = "cloud"      // 云资源
	MODEL_GROUP_DATABASE   = "database"   // 数据库
	MODEL_GROUP_MIDDLEWARE = "middleware" // 中间件
	MODEL_GROUP_CONTAINER  = "container"  // 容器服务
	MODEL_GROUP_STORAGE    = "storage"    // 存储设备
	MODEL_GROUP_SECURITY   = "security"   // 安全设备
	MODEL_GROUP_IAM        = "iam"        // 身份权限
	MODEL_GROUP_CUSTOM     = "custom"     // 自定义
)

// GetBuiltinModelGroups 获取预置模型分组
func GetBuiltinModelGroups() []ModelGroup {
	return []ModelGroup{
		{
			UID:       MODEL_GROUP_HOST,
			Name:      "主机管理",
			Icon:      "server",
			SortOrder: 1,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_CLOUD,
			Name:      "云资源",
			Icon:      "cloud",
			SortOrder: 2,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_NETWORK,
			Name:      "网络设备",
			Icon:      "network",
			SortOrder: 3,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_DATABASE,
			Name:      "数据库",
			Icon:      "database",
			SortOrder: 4,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_MIDDLEWARE,
			Name:      "中间件",
			Icon:      "middleware",
			SortOrder: 5,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_CONTAINER,
			Name:      "容器服务",
			Icon:      "container",
			SortOrder: 6,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_STORAGE,
			Name:      "存储设备",
			Icon:      "storage",
			SortOrder: 7,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_SECURITY,
			Name:      "安全设备",
			Icon:      "security",
			SortOrder: 8,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_IAM,
			Name:      "身份权限",
			Icon:      "user",
			SortOrder: 9,
			IsBuiltin: true,
		},
		{
			UID:       MODEL_GROUP_CUSTOM,
			Name:      "自定义",
			Icon:      "custom",
			SortOrder: 100,
			IsBuiltin: true,
		},
	}
}
