package web

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// ModelHandler 模型HTTP处理器
type ModelHandler struct {
	svc service.ModelService
}

// NewModelHandler 创建模型处理器
func NewModelHandler(svc service.ModelService) *ModelHandler {
	return &ModelHandler{svc: svc}
}

// RegisterRoutes 注册模型相关路由
func (h *ModelHandler) RegisterRoutes(r *gin.RouterGroup) {
	g := r.Group("/models")
	{
		g.POST("", h.Create)
		g.GET("/:uid", h.GetByUID)
		g.GET("", h.List)
		g.PUT("/:uid", h.Update)
		g.DELETE("/:uid", h.Delete)
	}
}

// CreateModelReq 创建模型请求
type CreateModelReq struct {
	UID          string `json:"uid" binding:"required"`
	Name         string `json:"name" binding:"required"`
	ModelGroupID int64  `json:"model_group_id"`
	ParentUID    string `json:"parent_uid"`
	Category     string `json:"category" binding:"required"`
	Level        int    `json:"level"`
	Icon         string `json:"icon"`
	Description  string `json:"description"`
	Provider     string `json:"provider"`
	Extensible   bool   `json:"extensible"`
}

// UpdateModelReq 更新模型请求
//
// 除 Extensible 外全部使用指针：更新接口是「部分更新」，
// 用零值当「未传」的哨兵会把「显式清空」也一起吞掉——
// 例如 model_group_id=0（移回未分组）、icon=""（清空图标）此前永远不生效。
type UpdateModelReq struct {
	Name         *string `json:"name"`
	ModelGroupID *int64  `json:"model_group_id"`
	Icon         *string `json:"icon"`
	Description  *string `json:"description"`
	Extensible   *bool   `json:"extensible"`
}

// ModelVO 模型视图对象
type ModelVO struct {
	ID           int64  `json:"id"`
	UID          string `json:"uid"`
	Name         string `json:"name"`
	ModelGroupID int64  `json:"model_group_id"`
	ParentUID    string `json:"parent_uid"`
	Category     string `json:"category"`
	Level        int    `json:"level"`
	Icon         string `json:"icon"`
	Description  string `json:"description"`
	Provider     string `json:"provider"`
	Extensible   bool   `json:"extensible"`
	CreateTime   int64  `json:"create_time"`
	UpdateTime   int64  `json:"update_time"`
}

// ModelListResp 模型列表响应
type ModelListResp struct {
	Models []ModelVO `json:"models"`
	Total  int64     `json:"total"`
}

// Create 创建模型
//
// 不再使用 ginx.WrapBody：它绑定失败时只写日志、不写响应体（客户端拿到空 200），
// 且业务拒绝一律 200，与本包 Delete/GetByUID 的状态码口径不一致。
func (h *ModelHandler) Create(ctx *gin.Context) {
	var req CreateModelReq
	if !bindJSONBody(ctx, &req) {
		return
	}

	model := domain.Model{
		UID:          req.UID,
		Name:         req.Name,
		ModelGroupID: req.ModelGroupID,
		ParentUID:    req.ParentUID,
		Category:     req.Category,
		Level:        req.Level,
		Icon:         req.Icon,
		Description:  req.Description,
		Provider:     req.Provider,
		Extensible:   req.Extensible,
	}

	id, err := h.svc.Create(ctx.Request.Context(), model)
	if err != nil {
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, Result(map[string]interface{}{"id": id}))
}

// GetByUID 根据UID获取模型
func (h *ModelHandler) GetByUID(ctx *gin.Context) {
	uid := strings.TrimSpace(ctx.Param("uid"))

	model, err := h.svc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		// 统一走 writeModelConfigError：状态码选择只在这里一处（见 model_config_errors.go）。
		// GetByUID 当前只会返回「未找到」（映射为 404），但若将来新增校验类哨兵，
		// 就地写死 404 会把 400 级拒绝报成「不存在」，正是这个 helper 要避免的分叉。
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(h.toVO(model)))
}

// List 获取模型列表
func (h *ModelHandler) List(ctx *gin.Context) {
	// 白名单校验放在最前面：未知参数直接报错，而不是被静默忽略后返回全量。
	// 实测（2026-11-19）：?keyword=zz-no-such-model 与 ?bogus=1 都返回全部 81 个模型。
	if rejectUnsupportedQuery(ctx, modelListParamSet) {
		return
	}
	provider := ctx.Query("provider")
	category := ctx.Query("category")
	parentUID := ctx.Query("parent_uid")
	levelStr := ctx.Query("level")
	// model_group_id：按模型分组过滤。
	// 此前该参数未被解析（filter.ModelGroupID 恒为 0，DAO 的过滤分支永不触发），
	// 导致传任意分组都返回全部模型。此处补齐解析。
	groupIDStr := ctx.Query("model_group_id")

	// extensible：是否可扩展。DAO 的 buildQuery 已支持（filter.Extensible != nil），
	// 但此前 handler 未解析，导致传任意值都返回全部模型
	// （实测 extensible=true / false 均返回 80，而实际全部为 true）。
	extensibleStr := ctx.Query("extensible")

	offset, limit, pagingErr := resolvePaging(ctx.Query("offset"), ctx.Query("limit"))
	if pagingErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, pagingErr))
		return
	}

	// level 与 model_group_id 此前解析失败即 0，而 0 表示「不过滤」——传错值等于
	// 静默丢掉筛选条件（实测 level=abc 返回全部 81 个模型，level=1 才是 41 个）。
	level64, levelErr := parseNonNegativeInt64(levelStr, "level")
	if levelErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, levelErr))
		return
	}
	level := int(level64)

	groupID, groupErr := parseNonNegativeInt64(groupIDStr, "model_group_id")
	if groupErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, groupErr))
		return
	}

	// extensible 取值非法必须报错：旧实现把无法识别的值当作「未传」，
	// 实测 extensible=zzz 返回全部 81 个模型。
	extensible, boolErr := parseOptionalBoolParam(extensibleStr, "extensible")
	if boolErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, boolErr))
		return
	}

	filter := domain.ModelFilter{
		Provider:     provider,
		Category:     category,
		ParentUID:    parentUID,
		Level:        level,
		ModelGroupID: groupID,
		Extensible:   extensible,
		Offset:       offset,
		Limit:        limit,
	}

	models, total, err := h.svc.List(ctx.Request.Context(), filter)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	modelVOs := make([]ModelVO, len(models))
	for i, m := range models {
		modelVOs[i] = h.toVO(m)
	}

	ctx.JSON(200, Result(ModelListResp{Models: modelVOs, Total: total}))
}

// Update 更新模型
func (h *ModelHandler) Update(ctx *gin.Context) {
	var req UpdateModelReq
	if !bindJSONBody(ctx, &req) {
		return
	}
	uid := strings.TrimSpace(ctx.Param("uid"))

	existing, err := h.svc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	// 指针字段：nil 表示「本次不改」，非 nil 表示「按传入值覆盖」，
	// 因此显式传空串/0 会被真正写回（配合服务层校验给出中文拒绝）。
	// 文本归一统一由 service（Model.Normalize）负责，这里不再就地 TrimSpace：
	// 与 model_group_handler 保持同一套「handler 只合并、service 归一」的约定。
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.ModelGroupID != nil {
		existing.ModelGroupID = *req.ModelGroupID
	}
	if req.Icon != nil {
		existing.Icon = *req.Icon
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Extensible != nil {
		existing.Extensible = *req.Extensible
	}

	err = h.svc.Update(ctx.Request.Context(), existing)
	if err != nil {
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, Result(nil))
}

// Delete 删除模型
func (h *ModelHandler) Delete(ctx *gin.Context) {
	uid := strings.TrimSpace(ctx.Param("uid"))

	err := h.svc.Delete(ctx.Request.Context(), uid)
	if err != nil {
		// 状态码由 modelConfigErrorStatus 统一决定（未找到 404 / 仍有实例 409），
		// 不再在本处用 switch 反查业务码。
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, Result(nil))
}

func (h *ModelHandler) toVO(model domain.Model) ModelVO {
	return ModelVO{
		ID:           model.ID,
		UID:          model.UID,
		Name:         model.Name,
		ModelGroupID: model.ModelGroupID,
		ParentUID:    model.ParentUID,
		Category:     model.Category,
		Level:        model.Level,
		Icon:         model.Icon,
		Description:  model.Description,
		Provider:     model.Provider,
		Extensible:   model.Extensible,
		CreateTime:   model.CreateTime.UnixMilli(),
		UpdateTime:   model.UpdateTime.UnixMilli(),
	}
}
