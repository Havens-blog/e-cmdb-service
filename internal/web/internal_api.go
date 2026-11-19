package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/gin-gonic/gin"
)

// InternalAPIHandler 内部服务间 API（供 e-cam-service 的 RemoteCMDBAdapter 调用）。
//
// 这些端点与 e-cam-service 的 port.CMDBPort 契约一一对应，是 CMDB 拆分为独立服务
// 后 servicetree 远程访问 CMDB 数据的服务端。挂在 /api/v1/cmdb/internal 前缀下，
// 仅内网网关可达；租户由调用方（e-cam-service，已在网关后完成会话校验）在请求体/
// 查询参数显式传入——这是服务间可信调用，不走 RequireTenant。
type InternalAPIHandler struct {
	repo    repository.InstanceRepository
	models  repository.ModelRepository
	topoSvc service.TopologyService
}

// NewInternalAPIHandler 创建内部 API 处理器。
func NewInternalAPIHandler(repo repository.InstanceRepository, models repository.ModelRepository, topoSvc service.TopologyService) *InternalAPIHandler {
	return &InternalAPIHandler{repo: repo, models: models, topoSvc: topoSvc}
}

// ---- wire DTO：与 e-cam-service port 包的结构镜像，JSON tag 为跨服务契约 ----

// InstanceDTO 实例的跨服务传输结构。
type InstanceDTO struct {
	ID         int64                  `json:"id"`
	ModelUID   string                 `json:"model_uid"`
	AssetID    string                 `json:"asset_id"`
	AssetName  string                 `json:"asset_name"`
	TenantID   int64                  `json:"tenant_id"`
	AccountID  int64                  `json:"account_id"`
	Attributes map[string]interface{} `json:"attributes"`
	CreateTime int64                  `json:"create_time"` // UnixMilli
	UpdateTime int64                  `json:"update_time"` // UnixMilli
}

// StatsDTO 统计结果的跨服务传输结构。
type StatsDTO struct {
	Total       int64          `json:"total"`
	ByAssetType []TypeCountDTO `json:"by_asset_type"`
	ByProvider  []TypeCountDTO `json:"by_provider"`
	ByBusiness  []TypeCountDTO `json:"by_business"`
}

// TypeCountDTO 分组计数。
type TypeCountDTO struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// IDsReq 批量 ID 请求体。
// TenantID 可选：>0 时按租户过滤，<=0 时保持历史行为（不按租户过滤），
// 以兼容尚未传 tenant_id 的 e-cam-service 调用方。
type IDsReq struct {
	IDs      []int64 `json:"ids"`
	TenantID int64   `json:"tenant_id"`
}

// RegisterRoutes 注册内部 API 路由。
func (h *InternalAPIHandler) RegisterRoutes(r *gin.RouterGroup) {
	g := r.Group("/internal")
	{
		g.POST("/instances/batch-get", h.BatchGet)
		g.GET("/instances/list", h.ListInstances)
		g.GET("/instances/unbound", h.ListUnbound)
		g.GET("/instances/unbound/count", h.CountUnbound)
		g.POST("/stats/by-ids", h.StatsByIDs)
		g.GET("/stats/all", h.StatsAll)
		g.GET("/stats/unbound", h.StatsUnbound)
		g.GET("/model-aliases", h.ModelAliases)
		g.GET("/topology/instance/:id", h.InstanceTopology)
	}
}

// ModelAliases GET /internal/model-aliases 返回「别名 → 模型 UID」归一映射，
// 供 e-cam-service 的 modelalias.RemoteResolver 定期拉取（启动 + 每 5 分钟），
// 在多云同步写实例前把 {provider}_{类型} 形式的 model_uid 归一到真实模型，
// 从源头消除孤儿实例（映射规则见 domain.BuildModelAliases）。
//
// 响应为 code/msg/data 信封（与 resolver 的解码契约一致；注意区别于本文件
// 其余端点的裸 JSON——那些对应 e-cam RemoteCMDBAdapter 的另一套契约）。
func (h *InternalAPIHandler) ModelAliases(c *gin.Context) {
	// Limit=0 → DAO 不设上限；模型表为配置级数据（当前 ~80 条），全量拉取
	models, err := h.models.List(c.Request.Context(), domain.ModelFilter{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "msg": err.Error(), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": domain.BuildModelAliases(models)})
}

// BatchGet 批量按 ID 查实例。
func (h *InternalAPIHandler) BatchGet(c *gin.Context) {
	var req IDsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	instances, err := h.repo.ListByIDs(c.Request.Context(), req.IDs, req.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": toInstanceDTOs(instances)})
}

// ListUnbound 查未绑定实例。
//
// 参数校验走 parseIntQuery（缺省保持历史行为、显式非法值返回 400），
// limit 经 clampInternalLimit 钳进 [1,500]：DAO 对 limit<=0 不加 $limit，
// 曾出现 `limit=abc` 静默变成 0 → 一次返回 20166 条、耗时 9.2 秒。
func (h *InternalAPIHandler) ListUnbound(c *gin.Context) {
	tenantID, _, errMsg := parseIntQuery(c, "tenant_id")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	offset, offsetPresent, errMsg := parseIntQuery(c, "offset")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	if offsetPresent && offset < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset 必须是不小于 0 的整数"})
		return
	}
	limit, limitPresent, errMsg := parseIntQuery(c, "limit")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	instances, err := h.repo.ListUnbound(c.Request.Context(), tenantID, offset, clampInternalLimit(limit, limitPresent))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": toInstanceDTOs(instances)})
}

// ListInstances 按模型/名称/云厂商列实例，供 opsagent 资源拓扑查询（CDN 域名、
// 云资源清单）。租户由调用方显式传参（S2S 无会话租户）；model_uid 为空时按
// asset_name 模糊匹配。provider 可选，过滤 attributes.provider（如 volcano/aliyun）。
// limit 缺省 100、上限 500；显式非法值（如 tenant_id=abc）返回 400，不静默兜底。
func (h *InternalAPIHandler) ListInstances(c *gin.Context) {
	modelUID := c.Query("model_uid")
	assetName := c.Query("asset_name")
	provider := c.Query("provider")
	assetID := c.Query("asset_id")
	tenantID, _, errMsg := parseIntQuery(c, "tenant_id")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	limit, limitPresent, errMsg := parseIntQuery(c, "limit")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	limit = clampInternalLimit(limit, limitPresent)
	filter := domain.InstanceFilter{
		ModelUID:  modelUID,
		AssetName: assetName,
		AssetID:   assetID,
		TenantID:  tenantID,
		Limit:     limit,
	}
	if provider != "" {
		filter.Attributes = map[string]interface{}{"provider": provider}
	}
	instances, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	total, err := h.repo.Count(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": toInstanceDTOs(instances), "total": total})
}

// CountUnbound 统计未绑定实例数。
func (h *InternalAPIHandler) CountUnbound(c *gin.Context) {
	tenantID, _, errMsg := parseIntQuery(c, "tenant_id")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	count, err := h.repo.CountUnbound(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// StatsByIDs 按 ID 列表聚合统计。
func (h *InternalAPIHandler) StatsByIDs(c *gin.Context) {
	var req IDsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.repo.AggregateStatsByIDs(c.Request.Context(), req.IDs, req.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toStatsDTO(result))
}

// StatsAll 聚合统计全部实例。
func (h *InternalAPIHandler) StatsAll(c *gin.Context) {
	tenantID, _, errMsg := parseIntQuery(c, "tenant_id")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	result, err := h.repo.AggregateAllStats(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toStatsDTO(result))
}

// StatsUnbound 聚合统计未绑定实例。
func (h *InternalAPIHandler) StatsUnbound(c *gin.Context) {
	tenantID, _, errMsg := parseIntQuery(c, "tenant_id")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	result, err := h.repo.AggregateUnboundStats(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toStatsDTO(result))
}

// internalListDefaultLimit / internalListMaxLimit 是内部 S2S 列表端点的单次取数上限。
//
// 取值与 Web 侧 maxListLimit 对齐（500），因为调用方（e-cam servicetree 未绑定弹窗、
// opsagent 资源拓扑）都按「一次最多 500 条」设计；超过上限一律钳到上限，
// 而不是回落到默认值，避免「要 500 条只给 100 条」这种静默降级。
const (
	internalListDefaultLimit = 100
	internalListMaxLimit     = 500
)

// parseIntQuery 解析可选的整数查询参数，返回 (value, present, errMsg)。
//
//   - present=false（参数缺省或空串）：调用方沿用历史默认值，保持 S2S 兼容；
//   - errMsg != ""（参数显式传入但无法解析）：调用方必须返回 400，**不得**按 0 兜底。
//
// 为什么不能像旧实现那样 `v, _ := strconv.ParseInt(...)` 静默吞错：
// 对 tenant_id 而言 0 表示「不加租户过滤」（见 instanceDAO.buildQuery 的
// `if filter.TenantID != 0`），于是 tenant_id=abc 会退化成跨租户全量读取。
// 2026-11-19 线上实测 `/internal/instances/list?tenant_id=abc&limit=5`
// 返回 total=21498（租户 1 的 240 条 + 租户 3 的 21258 条），而 tenant_id=1/3
// 分别只返回 240 / 21258 —— 一个拼错的参数就能把全部租户的数据读出去，属 fail-open。
func parseIntQuery(c *gin.Context, key string) (value int64, present bool, errMsg string) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, false, ""
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, true, key + " 必须是整数（收到 " + strconv.Quote(raw) + "）"
	}
	return v, true, ""
}

// clampInternalLimit 把 limit 钳进 [1, internalListMaxLimit]：
// 缺省、<=0 或非法（理论上已由 parseIntQuery 拦下）统一取默认值 100，
// 超过上限钳到 500。DAO 对 limit<=0 的解释是「不加 $limit」，即导出全集合
// （实测未绑定实例 20166 条、9.2 秒），所以这里绝不能把 0 透传下去。
func clampInternalLimit(limit int64, present bool) int64 {
	if !present || limit <= 0 {
		return internalListDefaultLimit
	}
	if limit > internalListMaxLimit {
		return internalListMaxLimit
	}
	return limit
}

// InstanceTopology GET /internal/topology/instance/:id 返回以指定实例为起点的
// 实例拓扑图（nodes+edges）。与 /api/v1/cmdb/topology/instance/:id 共用同一个
// topologySvc，但走服务间可信调用：租户由调用方在 query 显式传入（tenant_id），
// 与本组其余端点契约一致，供 OpsAgent 等机器调用方查询资源拓扑。
func (h *InternalAPIHandler) InstanceTopology(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance id"})
		return
	}
	tenantID, _, errMsg := parseIntQuery(c, "tenant_id")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	depth, _, errMsg := parseIntQuery(c, "depth")
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	query := domain.TopologyQuery{
		InstanceID: id,
		TenantID:   tenantID,
		Depth:      int(depth),
		Direction:  c.DefaultQuery("direction", "both"),
	}
	graph, gerr := h.topoSvc.GetInstanceTopology(c.Request.Context(), query)
	if gerr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": graph.Nodes, "edges": graph.Edges})
}
