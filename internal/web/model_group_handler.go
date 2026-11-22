package web

import (
	"strings"

	"github.com/Havens-blog/e-common-go/ginx"
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
		groupRouter.POST("", ginx.WrapBody[CreateModelGroupReq](h.Create))
		groupRouter.GET("", h.List)
		groupRouter.GET("/with-models", h.ListWithModels)
		groupRouter.GET("/:uid", h.GetByUID)
		groupRouter.PUT("/:uid", ginx.WrapBody[UpdateModelGroupReq](h.Update))
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
type UpdateModelGroupReq struct {
	// Name 用指针区分「未传」与「显式传空」：未传保留旧值（部分更新），
	// 显式传空/纯空白则仍走服务层校验返回 400，而不是被静默忽略（评审 #1 后续补验）。
	Name        *string `json:"name"`
	Icon        string  `json:"icon"`
	SortOrder   int     `json:"sort_order"`
	Description string  `json:"description"`
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
func (h *ModelGroupHandler) Create(ctx *gin.Context, req CreateModelGroupReq) (ginx.Result, error) {
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
		if result, ok := modelConfigErrorResult(err); ok {
			return result, nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]any{"id": id}), nil
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
		if err == errs.ErrModelGroupNotFound {
			ctx.JSON(404, ErrorResult(errs.ModelGroupNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(h.toVO(group)))
}

// Update 更新模型分组
func (h *ModelGroupHandler) Update(ctx *gin.Context, req UpdateModelGroupReq) (ginx.Result, error) {
	uid := strings.TrimSpace(ctx.Param("uid"))

	// 与模型更新同口径：先取回旧值再按非空字段合并（评审 #1）。
	// UpdateModelGroupReq 的所有字段在 OpenAPI 里都是可选的，
	// 若整条替换，只传 {"description":...} 的部分更新会把分组名冲成空串
	// （服务层现在会拒绝空名称，于是这类合法请求变成 400）。
	existing, err := h.svc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		if result, ok := modelConfigErrorResult(err); ok {
			return result, nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Icon != "" {
		existing.Icon = req.Icon
	}
	if req.SortOrder != 0 {
		existing.SortOrder = req.SortOrder
	}
	if req.Description != "" {
		existing.Description = req.Description
	}

	err = h.svc.Update(ctx.Request.Context(), existing)
	if err != nil {
		if result, ok := modelConfigErrorResult(err); ok {
			return result, nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// Delete 删除模型分组
func (h *ModelGroupHandler) Delete(ctx *gin.Context) {
	uid := strings.TrimSpace(ctx.Param("uid"))

	err := h.svc.Delete(ctx.Request.Context(), uid)
	if err != nil {
		if result, ok := modelConfigErrorResult(err); ok {
			// 未找到 → 404；「内置不可删」保持 400；「组内仍有模型」是 409（资源状态冲突）。
			status := 409
			switch result.Code {
			case errs.ModelGroupNotFound.Code:
				status = 404
			case errs.CannotDeleteBuiltin.Code:
				status = 400
			}
			ctx.JSON(status, result)
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(nil))
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
