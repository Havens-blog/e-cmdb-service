package web

import (
	"strconv"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
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
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	SortOrder   int    `json:"sort_order"`
	Description string `json:"description"`
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
	group := domain.ModelGroup{
		UID:         req.UID,
		Name:        req.Name,
		Icon:        req.Icon,
		SortOrder:   req.SortOrder,
		Description: req.Description,
	}

	id, err := h.svc.Create(ctx.Request.Context(), group)
	if err != nil {
		if err == errs.ErrModelGroupExists {
			return ErrorResultWithMsg(errs.ModelGroupExists, "model group already exists"), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]any{"id": id}), nil
}

// List 获取模型分组列表
func (h *ModelGroupHandler) List(ctx *gin.Context) {
	offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "100"))

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
	uid := ctx.Param("uid")

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
	uid := ctx.Param("uid")

	group := domain.ModelGroup{
		UID:         uid,
		Name:        req.Name,
		Icon:        req.Icon,
		SortOrder:   req.SortOrder,
		Description: req.Description,
	}

	err := h.svc.Update(ctx.Request.Context(), group)
	if err != nil {
		if err == errs.ErrModelGroupNotFound {
			return ErrorResult(errs.ModelGroupNotFound), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// Delete 删除模型分组
func (h *ModelGroupHandler) Delete(ctx *gin.Context) {
	uid := ctx.Param("uid")

	err := h.svc.Delete(ctx.Request.Context(), uid)
	if err != nil {
		if err == errs.ErrModelGroupNotFound {
			ctx.JSON(404, ErrorResult(errs.ModelGroupNotFound))
			return
		}
		if err == errs.ErrCannotDeleteBuiltin {
			ctx.JSON(400, ErrorResult(errs.CannotDeleteBuiltin))
			return
		}
		if err == errs.ErrGroupHasModels {
			ctx.JSON(400, ErrorResult(errs.GroupHasModels))
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
