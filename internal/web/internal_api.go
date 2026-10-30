package web

import (
	"net/http"
	"strconv"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gin-gonic/gin"
)

// InternalAPIHandler 内部服务间 API（供 e-cam-service 的 RemoteCMDBAdapter 调用）。
//
// 这些端点与 e-cam-service 的 port.CMDBPort 契约一一对应，是 CMDB 拆分为独立服务
// 后 servicetree 远程访问 CMDB 数据的服务端。挂在 /api/v1/cmdb/internal 前缀下，
// 仅内网网关可达；租户由调用方（e-cam-service，已在网关后完成会话校验）在请求体/
// 查询参数显式传入——这是服务间可信调用，不走 RequireTenant。
type InternalAPIHandler struct {
	repo   repository.InstanceRepository
	models repository.ModelRepository
}

// NewInternalAPIHandler 创建内部 API 处理器。
func NewInternalAPIHandler(repo repository.InstanceRepository, models repository.ModelRepository) *InternalAPIHandler {
	return &InternalAPIHandler{repo: repo, models: models}
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
func (h *InternalAPIHandler) ListUnbound(c *gin.Context) {
	tenantID := queryInt64(c, "tenant_id")
	offset := queryInt64(c, "offset")
	limit := queryInt64(c, "limit")
	instances, err := h.repo.ListUnbound(c.Request.Context(), tenantID, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": toInstanceDTOs(instances)})
}

// ListInstances 按模型/名称/云厂商列实例，供 opsagent 资源拓扑查询（CDN 域名、
// 云资源清单）。租户由调用方显式传参（S2S 无会话租户）；model_uid 为空时按
// asset_name 模糊匹配。provider 可选，过滤 attributes.provider（如 volcano/aliyun）。
// limit 缺省 100、上限 500。
func (h *InternalAPIHandler) ListInstances(c *gin.Context) {
	modelUID := c.Query("model_uid")
	assetName := c.Query("asset_name")
	provider := c.Query("provider")
	tenantID := queryInt64(c, "tenant_id")
	limit := queryInt64(c, "limit")
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	filter := domain.InstanceFilter{
		ModelUID:  modelUID,
		AssetName: assetName,
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
	tenantID := queryInt64(c, "tenant_id")
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
	tenantID := queryInt64(c, "tenant_id")
	result, err := h.repo.AggregateAllStats(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toStatsDTO(result))
}

// StatsUnbound 聚合统计未绑定实例。
func (h *InternalAPIHandler) StatsUnbound(c *gin.Context) {
	tenantID := queryInt64(c, "tenant_id")
	result, err := h.repo.AggregateUnboundStats(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toStatsDTO(result))
}

func queryInt64(c *gin.Context, key string) int64 {
	v, _ := strconv.ParseInt(c.Query(key), 10, 64)
	return v
}
