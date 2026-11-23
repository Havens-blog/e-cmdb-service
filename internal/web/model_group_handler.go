package web

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// ModelGroupHandler 模型分组HTTP处理器
type ModelGroupHandler struct {
	svc service.ModelGroupService
}

// NewModelGroupHandler 创建模型分组处理器
func NewModelGroupHandler(svc service.ModelGroupService) *ModelGroupHandler {
	return &ModelGroupHandler{svc: svc}
}

// RegisterRoutes 注册模型分组路由
func (h *ModelGroupHandler) RegisterRoutes(r *gin.RouterGroup) {
	groupRouter := r.Group("/model-groups")
	{
		groupRouter.POST("", h.Create)
		groupRouter.GET("", h.List)
		groupRouter.GET("/with-models", h.ListWithModels)
		groupRouter.GET("/:uid", h.GetByUID)
		groupRouter.PUT("/:uid", h.Update)
		groupRouter.DELETE("/:uid", h.Delete)
		groupRouter.POST("/init", h.InitBuiltinGroups)
	}
}

// CreateModelGroupReq 创建模型分组请求
type CreateModelGroupReq struct {
	UID         string `json:"uid" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Icon        string `json:"icon"`
	SortOrder   int    `json:"sort_order"`
	Description string `json:"description"`
}

// UpdateModelGroupReq 更新模型分组请求
//
// 全部用指针（Name 已有，Icon/SortOrder/Description 本次补上）：
// 部分更新若用零值当「未传」哨兵，`sort_order: 0`（置顶）与
// `description: ""`（清空描述）都会被静默忽略，字段永远保留旧值。
type UpdateModelGroupReq struct {
	// Name 用指针区分「未传」与「显式传空」：未传保留旧值（部分更新），
	// 显式传空/纯空白则仍走服务层校验返回 400，而不是被静默忽略。
	Name        *string `json:"name"`
	Icon        *string `json:"icon"`
	SortOrder   *int    `json:"sort_order"`
	Description *string `json:"description"`
}

// ModelGroupVO 模型分组视图对象
type ModelGroupVO struct {
	ID          int64  `json:"id"`
	UID         string `json:"uid"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	SortOrder   int    `json:"sort_order"`
	IsBuiltin   bool   `json:"is_builtin"`
	Description string `json:"description"`
	CreateTime  int64  `json:"create_time"`
	UpdateTime  int64  `json:"update_time"`
}

// ModelGroupWithModelsVO 带模型列表的分组视图对象
type ModelGroupWithModelsVO struct {
	ModelGroupVO
	Models []ModelVO `json:"models"`
}

// Create 创建模型分组
func (h *ModelGroupHandler) Create(ctx *gin.Context) {
	var req CreateModelGroupReq
	if !bindJSONBody(ctx, &req) {
		return
	}

	// 裁剪首尾空白：gin 的 binding:"required" 不拦纯空白，实测 " " 能建出一个没有标识的空分组。
	group := domain.ModelGroup{
		UID:         strings.TrimSpace(req.UID),
		Name:        strings.TrimSpace(req.Name),
		Icon:        req.Icon,
		SortOrder:   req.SortOrder,
		Description: req.Description,
	}

	id, err := h.svc.Create(ctx.Request.Context(), group)
	if err != nil {
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, Result(map[string]any{"id": id}))
}

// List 获取模型分组列表
func (h *ModelGroupHandler) List(ctx *gin.Context) {
	if rejectUnsupportedQuery(ctx, modelGroupListParamSet) {
		return
	}
	offset, limit, pagingErr := resolvePagingWithin(ctx.Query("offset"), ctx.Query("limit"), attributeListLimit, maxListLimit)
	if pagingErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, pagingErr))
		return
	}

	filter := domain.ModelGroupFilter{
		Offset: offset,
		Limit:  limit,
	}

	groups, total, err := h.svc.List(ctx.Request.Context(), filter)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	vos := make([]ModelGroupVO, len(groups))
	for i, g := range groups {
		vos[i] = h.toVO(g)
	}

	ctx.JSON(200, Result(map[string]any{"groups": vos, "total": total}))
}

// ListWithModels 获取分组列表及其下的模型
func (h *ModelGroupHandler) ListWithModels(ctx *gin.Context) {
	groupsWithModels, err := h.svc.ListWithModels(ctx.Request.Context())
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	vos := make([]ModelGroupWithModelsVO, len(groupsWithModels))
	for i, g := range groupsWithModels {
		modelVOs := make([]ModelVO, len(g.Models))
		for j, m := range g.Models {
			modelVOs[j] = ModelVO{
				ID:           m.ID,
				UID:          m.UID,
				Name:         m.Name,
				ModelGroupID: m.ModelGroupID,
				ParentUID:    m.ParentUID,
				Category:     m.Category,
				Level:        m.Level,
				Icon:         m.Icon,
				Description:  m.Description,
				Provider:     m.Provider,
				Extensible:   m.Extensible,
				CreateTime:   m.CreateTime.UnixMilli(),
				UpdateTime:   m.UpdateTime.UnixMilli(),
			}
		}
		vos[i] = ModelGroupWithModelsVO{
			ModelGroupVO: h.toVO(g.ModelGroup),
			Models:       modelVOs,
		}
	}

	ctx.JSON(200, Result(map[string]any{"groups": vos}))
}

// GetByUID 根据UID获取模型分组
func (h *ModelGroupHandler) GetByUID(ctx *gin.Context) {
	uid := strings.TrimSpace(ctx.Param("uid"))

	group, err := h.svc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		// 与模型侧 GetByUID 同口径：走统一翻译（原先这里用 `err == 哨兵` +
		// ErrorResult 返回英文 "model group not found"，是本包唯一漏改的入口）。
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, Result(h.toVO(group)))
}

// Update 更新模型分组
func (h *ModelGroupHandler) Update(ctx *gin.Context) {
	var req UpdateModelGroupReq
	if !bindJSONBody(ctx, &req) {
		return
	}
	uid := strings.TrimSpace(ctx.Param("uid"))

	// 与模型更新同口径：先取回旧值再按「是否传入」合并。
	// UpdateModelGroupReq 所有字段在 OpenAPI 里都是可选的，
	// 若整条替换，只传 {"description":...} 的部分更新会把分组名冲成空串。
	existing, err := h.svc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	// 文本归一（uid/name/icon/description 的 TrimSpace）统一由
	// service.normalizeModelGroup 负责，handler 只做「是否传入」的合并。
	// 两层各裁一半会让「未修改 vs 显式改空」的判定分叉：早前 handler 只裁 Name，
	// 而 service 裁 Icon/Description，于是 icon="   " 在 handler 看上去是「显式赋值」
	// 到 service 才被裁成空串，两处对同一输入的理解不一致。
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Icon != nil {
		existing.Icon = *req.Icon
	}
	if req.SortOrder != nil {
		existing.SortOrder = *req.SortOrder
	}
	if req.Description != nil {
		existing.Description = *req.Description
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

// Delete 删除模型分组
func (h *ModelGroupHandler) Delete(ctx *gin.Context) {
	uid := strings.TrimSpace(ctx.Param("uid"))

	err := h.svc.Delete(ctx.Request.Context(), uid)
	if err != nil {
		// 状态码统一由 modelConfigErrorStatus 决定：未找到 404、组内仍有模型 409、
		// 内置不可删 400。原先在这里 switch 业务码，新增哨兵错误会落进默认分支变 409。
		if writeModelConfigError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, Result(nil))
}

// InitBuiltinGroups 初始化内置分组
func (h *ModelGroupHandler) InitBuiltinGroups(ctx *gin.Context) {
	err := h.svc.InitBuiltinGroups(ctx.Request.Context())
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(map[string]any{"message": "builtin groups initialized"}))
}

func (h *ModelGroupHandler) toVO(g domain.ModelGroup) ModelGroupVO {
	return ModelGroupVO{
		ID:          g.ID,
		UID:         g.UID,
		Name:        g.Name,
		Icon:        g.Icon,
		SortOrder:   g.SortOrder,
		IsBuiltin:   g.IsBuiltin,
		Description: g.Description,
		CreateTime:  g.CreateTime.UnixMilli(),
		UpdateTime:  g.UpdateTime.UnixMilli(),
	}
}
