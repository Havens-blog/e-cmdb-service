package web

import (
	"strconv"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

// InstanceHandler 资产实例HTTP处理器
type InstanceHandler struct {
	svc service.InstanceService
}

// NewInstanceHandler 创建实例处理器
func NewInstanceHandler(svc service.InstanceService) *InstanceHandler {
	return &InstanceHandler{svc: svc}
}

// RegisterRoutes 注册实例相关路由
func (h *InstanceHandler) RegisterRoutes(r *gin.RouterGroup) {
	// 整个 /instances 子组都要求已选定租户：domain.Instance 与 InstanceFilter 都带
	// TenantID，且 dao/instance.go:280 是 `if filter.TenantID != 0 { 加租户谓词 }` ——
	// 租户为 0（eiam 的「等待选择租户」临时凭证）时谓词被丢弃，List 会返回全部租户的
	// 实例。按设计 §4.3，0 必须在中间件边界拒绝，不得进入 DAO。
	//
	// 挂在子组而非 cmdb 组上：cmdb 是混合组，/models、/model-groups、
	// /models/:uid/attributes、/model-relations 的领域类型均不含 TenantID
	// （模型元数据本身无租户维度），整组挂会把它们一并变成 403。
	g := r.Group("/instances")
	g.Use(middleware.RequireTenant(elog.DefaultLogger))
	{
		g.POST("", ginx.WrapBody[CreateInstanceReq](h.Create))
		g.POST("/batch", ginx.WrapBody[CreateBatchInstanceReq](h.CreateBatch))
		g.POST("/upsert", ginx.WrapBody[UpsertInstanceReq](h.Upsert))
		g.POST("/upsert-batch", ginx.WrapBody[UpsertBatchInstanceReq](h.UpsertBatch))
		g.GET("/:id", h.GetByID)
		g.GET("", h.List)
		g.PUT("/:id", ginx.WrapBody[UpdateInstanceReq](h.Update))
		g.DELETE("/:id", h.Delete)
	}
}

// CreateInstanceReq 创建实例请求
type CreateInstanceReq struct {
	ModelUID   string                 `json:"model_uid" binding:"required"`
	AssetID    string                 `json:"asset_id" binding:"required"`
	AssetName  string                 `json:"asset_name"`
	AccountID  int64                  `json:"account_id"`
	Attributes map[string]interface{} `json:"attributes"`
}

// CreateBatchInstanceReq 批量创建实例请求
type CreateBatchInstanceReq struct {
	Instances []CreateInstanceReq `json:"instances" binding:"required"`
}

// UpsertInstanceReq 更新或插入实例请求
type UpsertInstanceReq struct {
	ModelUID   string                 `json:"model_uid" binding:"required"`
	AssetID    string                 `json:"asset_id" binding:"required"`
	AssetName  string                 `json:"asset_name"`
	AccountID  int64                  `json:"account_id"`
	Attributes map[string]interface{} `json:"attributes"`
}

// UpsertBatchInstanceReq 批量更新或插入实例请求
type UpsertBatchInstanceReq struct {
	Instances []UpsertInstanceReq `json:"instances" binding:"required"`
}

// UpdateInstanceReq 更新实例请求
type UpdateInstanceReq struct {
	AssetName  string                 `json:"asset_name"`
	Attributes map[string]interface{} `json:"attributes"`
}

// InstanceVO 实例视图对象
type InstanceVO struct {
	ID         int64                  `json:"id"`
	ModelUID   string                 `json:"model_uid"`
	AssetID    string                 `json:"asset_id"`
	AssetName  string                 `json:"asset_name"`
	TenantID   int64                  `json:"tenant_id"`
	AccountID  int64                  `json:"account_id"`
	Attributes map[string]interface{} `json:"attributes"`
	CreateTime int64                  `json:"create_time"`
	UpdateTime int64                  `json:"update_time"`
}

// InstanceListResp 实例列表响应
type InstanceListResp struct {
	Instances []InstanceVO `json:"instances"`
	Total     int64        `json:"total"`
}

// Create 创建实例
func (h *InstanceHandler) Create(ctx *gin.Context, req CreateInstanceReq) (ginx.Result, error) {
	instance := domain.Instance{
		ModelUID:   req.ModelUID,
		AssetID:    req.AssetID,
		AssetName:  req.AssetName,
		TenantID:   middleware.GetTenantID(ctx),
		AccountID:  req.AccountID,
		Attributes: req.Attributes,
	}

	id, err := h.svc.Create(ctx.Request.Context(), instance)
	if err != nil {
		if err == errs.ErrModelNotFound {
			return ErrorResult(errs.ModelNotFound), nil
		}
		if err == errs.ErrInstanceExists {
			return ErrorResultWithMsg(errs.ParamsError, "instance already exists"), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"id": id}), nil
}

// CreateBatch 批量创建实例
func (h *InstanceHandler) CreateBatch(ctx *gin.Context, req CreateBatchInstanceReq) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	instances := make([]domain.Instance, len(req.Instances))
	for i, r := range req.Instances {
		instances[i] = domain.Instance{
			ModelUID:   r.ModelUID,
			AssetID:    r.AssetID,
			AssetName:  r.AssetName,
			TenantID:   tenantID,
			AccountID:  r.AccountID,
			Attributes: r.Attributes,
		}
	}

	count, err := h.svc.CreateBatch(ctx.Request.Context(), instances)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"count": count}), nil
}

// Upsert 更新或插入实例
func (h *InstanceHandler) Upsert(ctx *gin.Context, req UpsertInstanceReq) (ginx.Result, error) {
	instance := domain.Instance{
		ModelUID:   req.ModelUID,
		AssetID:    req.AssetID,
		AssetName:  req.AssetName,
		TenantID:   middleware.GetTenantID(ctx),
		AccountID:  req.AccountID,
		Attributes: req.Attributes,
	}

	err := h.svc.Upsert(ctx.Request.Context(), instance)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// UpsertBatch 批量更新或插入实例
func (h *InstanceHandler) UpsertBatch(ctx *gin.Context, req UpsertBatchInstanceReq) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	instances := make([]domain.Instance, len(req.Instances))
	for i, r := range req.Instances {
		instances[i] = domain.Instance{
			ModelUID:   r.ModelUID,
			AssetID:    r.AssetID,
			AssetName:  r.AssetName,
			TenantID:   tenantID,
			AccountID:  r.AccountID,
			Attributes: r.Attributes,
		}
	}

	err := h.svc.UpsertBatch(ctx.Request.Context(), instances)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// GetByID 根据ID获取实例
func (h *InstanceHandler) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	instance, err := h.svc.GetByID(ctx.Request.Context(), middleware.GetTenantID(ctx), id)
	if err != nil {
		if err == errs.ErrInstanceNotFound {
			ctx.JSON(404, ErrorResult(errs.InstanceNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(h.toVO(instance)))
}

// List 获取实例列表
func (h *InstanceHandler) List(ctx *gin.Context) {
	modelUID := ctx.Query("model_uid")
	tenantID := middleware.GetTenantID(ctx)
	accountIDStr := ctx.Query("account_id")
	assetName := ctx.Query("asset_name")
	status := ctx.Query("status")
	region := ctx.Query("region")
	// provider：云厂商。前端实例列表有「云平台」下拉框并传递该参数，
	// 但此前后端未读取，导致筛选完全无效（实测 2026-10-31：
	// provider=aliyun 与 provider=zzz-no-such 均返回全量 33047 条）。
	// 云厂商在实例文档里是 attributes.provider 嵌套字段，
	// 与 status / region 同属「动态属性过滤」，故一并放入 attributes。
	provider := ctx.Query("provider")
	// asset_id：资产 ID 精确匹配。
	//
	// 此前该参数**完全未被读取**，传了会被静默忽略并返回全量。
	// 实测后果（2026-11-01）：调用方按 asset_id 定位单条记录后执行删除，
	// 实际命中的是全量列表的前 N 条，造成 10 条真实实例被误删。
	// 资产 ID 是唯一标识，支持精确匹配既符合直觉，也消除了这个陷阱。
	assetID := ctx.Query("asset_id")

	offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	var accountID int64
	if accountIDStr != "" {
		accountID, _ = strconv.ParseInt(accountIDStr, 10, 64)
	}

	attributes := make(map[string]interface{})
	if status != "" {
		attributes["status"] = status
	}
	if region != "" {
		attributes["region"] = region
	}
	if provider != "" {
		attributes["provider"] = provider
	}

	filter := domain.InstanceFilter{
		ModelUID:   modelUID,
		TenantID:   tenantID,
		AccountID:  accountID,
		AssetName:  assetName,
		AssetID:    assetID,
		Attributes: attributes,
		Offset:     int64(offset),
		Limit:      int64(limit),
	}

	instances, total, err := h.svc.List(ctx.Request.Context(), filter)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	instanceVOs := make([]InstanceVO, len(instances))
	for i, inst := range instances {
		instanceVOs[i] = h.toVO(inst)
	}

	ctx.JSON(200, Result(InstanceListResp{Instances: instanceVOs, Total: total}))
}

// Update 更新实例
func (h *InstanceHandler) Update(ctx *gin.Context, req UpdateInstanceReq) (ginx.Result, error) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return ErrorResult(errs.ParamsError), nil
	}

	tenantID := middleware.GetTenantID(ctx)
	existing, err := h.svc.GetByID(ctx.Request.Context(), tenantID, id)
	if err != nil {
		if err == errs.ErrInstanceNotFound {
			return ErrorResult(errs.InstanceNotFound), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	if req.AssetName != "" {
		existing.AssetName = req.AssetName
	}
	if req.Attributes != nil {
		if existing.Attributes == nil {
			existing.Attributes = make(map[string]interface{})
		}
		for k, v := range req.Attributes {
			existing.Attributes[k] = v
		}
	}

	err = h.svc.Update(ctx.Request.Context(), tenantID, existing)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// Delete 删除实例
func (h *InstanceHandler) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	err = h.svc.Delete(ctx.Request.Context(), middleware.GetTenantID(ctx), id)
	if err != nil {
		if err == errs.ErrInstanceNotFound {
			ctx.JSON(404, ErrorResult(errs.InstanceNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(nil))
}

func (h *InstanceHandler) toVO(instance domain.Instance) InstanceVO {
	return InstanceVO{
		ID:         instance.ID,
		ModelUID:   instance.ModelUID,
		AssetID:    instance.AssetID,
		AssetName:  instance.AssetName,
		TenantID:   instance.TenantID,
		AccountID:  instance.AccountID,
		Attributes: instance.Attributes,
		CreateTime: instance.CreateTime.UnixMilli(),
		UpdateTime: instance.UpdateTime.UnixMilli(),
	}
}
