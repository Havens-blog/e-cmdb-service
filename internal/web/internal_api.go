package web

import (
	"net/http"
	"strconv"

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
	repo repository.InstanceRepository
}

// NewInternalAPIHandler 创建内部 API 处理器。
func NewInternalAPIHandler(repo repository.InstanceRepository) *InternalAPIHandler {
	return &InternalAPIHandler{repo: repo}
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
		g.GET("/instances/unbound", h.ListUnbound)
		g.GET("/instances/unbound/count", h.CountUnbound)
		g.POST("/stats/by-ids", h.StatsByIDs)
		g.GET("/stats/all", h.StatsAll)
		g.GET("/stats/unbound", h.StatsUnbound)
	}
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
