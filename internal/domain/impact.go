package domain

// 影响分析（blast radius）的领域模型。
//
// 方向语义（待产品确认的合理默认）：
//   - downstream（下游影响）：沿出向关系（root 作为 source）到达的实例，即「root 故障会波及谁」。
//   - upstream（上游依赖）：沿入向关系（root 作为 target）到达的实例，即「root 依赖谁」。
const (
	// ImpactDirectionDownstream 下游影响方向。
	ImpactDirectionDownstream = "downstream"
	// ImpactDirectionUpstream 上游依赖方向。
	ImpactDirectionUpstream = "upstream"
)

// ImpactNode 影响分析节点。
// JSON tag 名与前端 e-cmdb-web/src/api/impact.ts 的 ImpactNode 字段严格对齐。
type ImpactNode struct {
	InstanceID int64  `json:"instance_id"`
	AssetID    string `json:"asset_id"`
	AssetName  string `json:"asset_name"`
	ModelUID   string `json:"model_uid"`
	Depth      int    `json:"depth"`
	Direction  string `json:"direction"` // root 为 ""，其余为 upstream|downstream
}

// ImpactEdge 影响分析边（关系）。
// source_id/target_id 始终为原始实例关系的 source/target，direction 标注本次遍历方向。
type ImpactEdge struct {
	SourceID        int64  `json:"source_id"`
	TargetID        int64  `json:"target_id"`
	RelationTypeUID string `json:"relation_type_uid"`
	Direction       string `json:"direction"` // upstream|downstream
}

// ImpactGraph 影响分析图。
type ImpactGraph struct {
	Root            ImpactNode   `json:"root"`
	Nodes           []ImpactNode `json:"nodes"`
	Edges           []ImpactEdge `json:"edges"`
	UpstreamCount   int          `json:"upstream_count"`
	DownstreamCount int          `json:"downstream_count"`
}
