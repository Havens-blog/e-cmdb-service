package web

import (
	"strconv"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
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
		g.POST("", ginx.WrapBody[CreateModelReq](h.Create))
		g.GET("/:uid", h.GetByUID)
		g.GET("", h.List)
		g.PUT("/:uid", ginx.WrapBody[UpdateModelReq](h.Update))
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
type UpdateModelReq struct {
	Name         string `json:"name"`
	ModelGroupID int64  `json:"model_group_id"`
	Icon         string `json:"icon"`
	Description  string `json:"description"`
	Extensible   *bool  `json:"extensible"`
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
func (h *ModelHandler) Create(ctx *gin.Context, req CreateModelReq) (ginx.Result, error) {
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
		if err == errs.ErrModelExists {
			return ErrorResultWithMsg(errs.ParamsError, "model already exists"), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"id": id}), nil
}

// GetByUID 根据UID获取模型
func (h *ModelHandler) GetByUID(ctx *gin.Context) {
	uid := ctx.Param("uid")

	model, err := h.svc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		if err == errs.ErrModelNotFound {
			ctx.JSON(404, ErrorResult(errs.ModelNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(h.toVO(model)))
}

// List 获取模型列表
func (h *ModelHandler) List(ctx *gin.Context) {
	provider := ctx.Query("provider")
	category := ctx.Query("category")
	parentUID := ctx.Query("parent_uid")
	levelStr := ctx.Query("level")
	// model_group_id：按模型分组过滤。
	// 此前该参数未被解析（filter.ModelGroupID 恒为 0，DAO 的过滤分支永不触发），
	// 导致传任意分组都返回全部模型。此处补齐解析。
	groupIDStr := ctx.Query("model_group_id")

	offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	var level int
	if levelStr != "" {
		level, _ = strconv.Atoi(levelStr)
	}

	var groupID int64
	if groupIDStr != "" {
		groupID, _ = strconv.ParseInt(groupIDStr, 10, 64)
	}

	filter := domain.ModelFilter{
		Provider:     provider,
		Category:     category,
		ParentUID:    parentUID,
		Level:        level,
		ModelGroupID: groupID,
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
func (h *ModelHandler) Update(ctx *gin.Context, req UpdateModelReq) (ginx.Result, error) {
	uid := ctx.Param("uid")

	existing, err := h.svc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		if err == errs.ErrModelNotFound {
			return ErrorResult(errs.ModelNotFound), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.ModelGroupID > 0 {
		existing.ModelGroupID = req.ModelGroupID
	}
	if req.Icon != "" {
		existing.Icon = req.Icon
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Extensible != nil {
		existing.Extensible = *req.Extensible
	}

	err = h.svc.Update(ctx.Request.Context(), existing)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// Delete 删除模型
func (h *ModelHandler) Delete(ctx *gin.Context) {
	uid := ctx.Param("uid")

	err := h.svc.Delete(ctx.Request.Context(), uid)
	if err != nil {
		if err == errs.ErrModelNotFound {
			ctx.JSON(404, ErrorResult(errs.ModelNotFound))
			return
		}
		if err == errs.ErrModelHasInstances {
			ctx.JSON(400, ErrorResult(errs.ModelHasInstances))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(nil))
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
