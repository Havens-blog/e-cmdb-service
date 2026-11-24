package web

import (
	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
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
		// 同步链路「记录但不阻断」写入的问题清单
		g.GET("/sync-issues", ginx.Wrap(h.SyncIssues))
		// 字段值不合规（类型/枚举）实时清单：方案 B 只警告不阻断，靠这里可见
		g.GET("/field-issues", ginx.Wrap(h.FieldIssues))
		// 强制重算质量分析（跳过 60s 缓存），供用户修复数据后立即核对
		g.POST("/refresh", ginx.Wrap(h.Refresh))
	}
}

// Overview 数据质量概览。
func (h *QualityHandler) Overview(ctx *gin.Context) (ginx.Result, error) {
	if res, bad := guardQueryParams(ctx, qualityOverviewParamSet); bad {
		return res, nil
	}
	tenantID := middleware.GetTenantID(ctx)

	data, err := h.svc.Overview(ctx.Request.Context(), tenantID)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}

// Issues 数据质量问题列表。
func (h *QualityHandler) Issues(ctx *gin.Context) (ginx.Result, error) {
	if res, bad := guardQueryParams(ctx, qualityIssuesParamSet); bad {
		return res, nil
	}
	tenantID := middleware.GetTenantID(ctx)
	// 取值非法必须报错：旧实现把未知 type 当成「没有该类问题」，
	// `?type=zzz` 返回 {"total":0}，与「真的没有问题」无法区分。
	// 用归一化后的返回值，不能把原始串透传给下游（`?type=%20orphan_instance` 会静默空集）。
	typ, enumErr := validateEnumParam(ctx.Query("type"), "type", qualityIssueTypes)
	if enumErr != "" {
		return ErrorResultWithMsg(errs.ParamsError, enumErr+"；字段级问题请改用 /api/v1/cmdb/quality/field-issues"), nil
	}
	offset, limit, pagingErr := resolvePaging(ctx.Query("offset"), ctx.Query("limit"))
	if pagingErr != "" {
		return ErrorResultWithMsg(errs.ParamsError, pagingErr), nil
	}

	data, err := h.svc.Issues(ctx.Request.Context(), tenantID, typ, offset, limit)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}

// Refresh 强制重算质量分析并返回最新概览。
//
// POST 语义：该操作会让缓存失效并触发一次全量扫描（实测约 5 秒），
// 不做幂等/GTE 优化 —— 用户显式要求「现在重新算一次」。
func (h *QualityHandler) Refresh(ctx *gin.Context) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	data, err := h.svc.Refresh(ctx.Request.Context(), tenantID)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}

// FieldIssues 字段值不合规（类型/枚举）实时清单。
//
// 支持 model_uid / issue_type 过滤（issue_type 取 field_type / field_enum），
// 分页参数沿用统一校验（resolvePaging）。
func (h *QualityHandler) FieldIssues(ctx *gin.Context) (ginx.Result, error) {
	if res, bad := guardQueryParams(ctx, qualityFieldIssuesParamSet); bad {
		return res, nil
	}
	tenantID := middleware.GetTenantID(ctx)
	modelUID := ctx.Query("model_uid")
	issueType, enumErr := validateEnumParam(ctx.Query("issue_type"), "issue_type", qualityFieldIssueTypes)
	if enumErr != "" {
		return ErrorResultWithMsg(errs.ParamsError, enumErr), nil
	}
	offset, limit, pagingErr := resolvePaging(ctx.Query("offset"), ctx.Query("limit"))
	if pagingErr != "" {
		return ErrorResultWithMsg(errs.ParamsError, pagingErr), nil
	}

	data, err := h.svc.FieldIssues(ctx.Request.Context(), tenantID, modelUID, issueType, offset, limit)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}

// SyncIssues 同步质量问题清单（记录但不阻断）。
func (h *QualityHandler) SyncIssues(ctx *gin.Context) (ginx.Result, error) {
	if res, bad := guardQueryParams(ctx, qualitySyncIssuesParamSet); bad {
		return res, nil
	}
	tenantID := middleware.GetTenantID(ctx)
	modelUID := ctx.Query("model_uid")
	assetID := ctx.Query("asset_id")
	issueType, enumErr := validateEnumParam(ctx.Query("issue_type"), "issue_type", syncIssueTypes)
	if enumErr != "" {
		return ErrorResultWithMsg(errs.ParamsError, enumErr), nil
	}
	offset, limit, pagingErr := resolvePaging(ctx.Query("offset"), ctx.Query("limit"))
	if pagingErr != "" {
		return ErrorResultWithMsg(errs.ParamsError, pagingErr), nil
	}

	data, err := h.svc.SyncIssues(ctx.Request.Context(), tenantID, modelUID, issueType, assetID, offset, limit)
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
	if res, bad := guardQueryParams(ctx, qualityOrphanSourcesParamSet); bad {
		return res, nil
	}
	tenantID := middleware.GetTenantID(ctx)

	data, err := h.svc.OrphanSources(ctx.Request.Context(), tenantID)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(data), nil
}
