package domain

// TopologyNode 拓扑节点
type TopologyNode struct {
	ID         int64                  `json:"id"`
	ModelUID   string                 `json:"model_uid"`
	ModelName  string                 `json:"model_name"`
	AssetID    string                 `json:"asset_id"`
	AssetName  string                 `json:"asset_name"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Icon       string                 `json:"icon,omitempty"`
	Category   string                 `json:"category,omitempty"`
}

// TopologyEdge 拓扑边（关系）
type TopologyEdge struct {
	SourceID        int64  `json:"source_id"`
	TargetID        int64  `json:"target_id"`
	RelationTypeUID string `json:"relation_type_uid"`
	RelationName    string `json:"relation_name"`
	RelationType    string `json:"relation_type"`
}

// TopologyGraph 拓扑图
type TopologyGraph struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// TopologyQuery 拓扑查询条件
type TopologyQuery struct {
	InstanceID int64  // 起始实例ID
	ModelUID   string // 按模型过滤
	TenantID   int64  // 租户ID
	Depth      int    // 查询深度，默认1
	Direction  string // 查询方向: both, outgoing, incoming
}

// ModelTopologyNode 模型拓扑节点
type ModelTopologyNode struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Provider string `json:"provider"`
	Icon     string `json:"icon,omitempty"`
}

// ModelTopologyEdge 模型拓扑边
type ModelTopologyEdge struct {
	SourceModelUID string `json:"source_model_uid"`
	TargetModelUID string `json:"target_model_uid"`
	RelationUID    string `json:"relation_uid"`
	RelationName   string `json:"relation_name"`
	RelationType   string `json:"relation_type"`
}

// ModelTopologyGraph 模型拓扑图（展示模型间的关系定义）
type ModelTopologyGraph struct {
	Nodes []ModelTopologyNode `json:"nodes"`
	Edges []ModelTopologyEdge `json:"edges"`
}
