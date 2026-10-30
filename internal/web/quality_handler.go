package web

import (
	"strconv"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

// QualityHandler 数据质量 HTTP 处理器。
type QualityHandler struct {
	svc service.QualityService
}

// NewQualityHandler 创建数据质量处理器。
func NewQualityHandler(svc service.QualityService) *QualityHandler {
	return &QualityHandler{svc: svc}
}

// RegisterRoutes 注册数据质量相关路由。
//
// 数据质量以实例/实例关系为主体，属租户作用域，因此整个 /quality 子组
// 挂 RequireTenant：实例与关系列表都按会话租户过滤，租户为 0 时必须在此拒绝。
func (h *QualityHandler) RegisterRoutes(r *gin.RouterGroup) {
	g := r.Group("/quality")
	g.Use(middleware.RequireTenant(elog.DefaultLogger))
	{
		g.GET("/overview", ginx.Wrap(h.Overview))
		g.GET("/issues", ginx.Wrap(h.Issues))
		g.GET("/orphan-sources", ginx.Wrap(h.OrphanSources))
	}
}

// Overview 数据质量概览。
func (h *QualityHandler) Overview(ctx *gin.Context) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)

	data, err := h.svc.Overview(ctx.Request.Context(), tenantID)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}

// Issues 数据质量问题列表。
func (h *QualityHandler) Issues(ctx *gin.Context) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	typ := ctx.Query("type")
	offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	data, err := h.svc.Issues(ctx.Request.Context(), tenantID, typ, offset, limit)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}

// OrphanSources 孤儿实例来源分布（按缺失模型聚合）。
//
// 存在的意义：/quality/overview 只给一个 orphan_count 总数，
// /quality/issues?type=orphan_instance 给的是逐条明细（分页），
// 两者都无法回答「这些孤儿主要是哪些模型缺失造成的」。
// 本接口直接返回按 model_uid 聚合的降序列表，让运维知道该补齐哪些模型。
func (h *QualityHandler) OrphanSources(ctx *gin.Context) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)

	data, err := h.svc.OrphanSources(ctx.Request.Context(), tenantID)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}