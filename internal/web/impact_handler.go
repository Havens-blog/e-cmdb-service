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

// ImpactHandler 影响分析（blast radius）HTTP 处理器。
type ImpactHandler struct {
	svc service.ImpactService
}

// NewImpactHandler 创建影响分析处理器。
func NewImpactHandler(svc service.ImpactService) *ImpactHandler {
	return &ImpactHandler{svc: svc}
}

// RegisterRoutes 注册影响分析相关路由。
//
// 影响分析以实例/实例关系为主体，属租户作用域，因此整个 /impact 子组挂
// RequireTenant：遍历按会话租户隔离，租户为 0 时必须在此拒绝，不得进入 service/DAO。
func (h *ImpactHandler) RegisterRoutes(r *gin.RouterGroup) {
	g := r.Group("/impact")
	g.Use(middleware.RequireTenant(elog.DefaultLogger))
	{
		g.GET("", ginx.Wrap(h.GetImpact))
	}
}

// GetImpact 影响分析。
func (h *ImpactHandler) GetImpact(ctx *gin.Context) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)

	instanceIDStr := ctx.Query("instance_id")
	instanceID, err := strconv.ParseInt(instanceIDStr, 10, 64)
	if err != nil || instanceID <= 0 {
		return ErrorResult(errs.ParamsError), nil
	}

	depth, _ := strconv.Atoi(ctx.DefaultQuery("depth", "1"))

	graph, err := h.svc.GetImpact(ctx.Request.Context(), tenantID, instanceID, depth)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(graph), nil
}
