package web

import (
	"strconv"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
)

// AttributeHandler 属性HTTP处理器
type AttributeHandler struct {
	svc service.AttributeService
}

// NewAttributeHandler 创建属性处理器
func NewAttributeHandler(svc service.AttributeService) *AttributeHandler {
	return &AttributeHandler{svc: svc}
}

// RegisterRoutes 注册属性相关路由
func (h *AttributeHandler) RegisterRoutes(r *gin.RouterGroup) {
	// 字段类型
	r.GET("/field-types", h.GetFieldTypes)

	// 模型属性
	attrGroup := r.Group("/models/:uid/attributes")
	{
		attrGroup.POST("", ginx.WrapBody[CreateAttributeReq](h.CreateAttribute))
		attrGroup.GET("", h.ListAttributes)
		attrGroup.GET("/grouped", h.ListAttributesWithGroups)
		attrGroup.GET("/:id", h.GetAttribute)
		attrGroup.PUT("/:id", ginx.WrapBody[UpdateAttributeReq](h.UpdateAttribute))
		attrGroup.DELETE("/:id", h.DeleteAttribute)
	}

	// 属性分组
	groupGroup := r.Group("/models/:uid/attribute-groups")
	{
		groupGroup.POST("", ginx.WrapBody[CreateAttributeGroupReq](h.CreateAttributeGroup))
		groupGroup.GET("", h.ListAttributeGroups)
		groupGroup.GET("/:id", h.GetAttributeGroup)
		groupGroup.PUT("/:id", ginx.WrapBody[UpdateAttributeGroupReq](h.UpdateAttributeGroup))
		groupGroup.DELETE("/:id", h.DeleteAttributeGroup)
	}
}

// ========== Request/Response Types ==========

// CreateAttributeReq 创建属性请求
type CreateAttributeReq struct {
	FieldUID    string      `json:"field_uid" binding:"required"`
	FieldName   string      `json:"field_name" binding:"required"`
	FieldType   string      `json:"field_type" binding:"required"`
	GroupID     int64       `json:"group_id"`
	DisplayName string      `json:"display_name"`
	Display     bool        `json:"display"`
	Index       int         `json:"index"`
	Required    bool        `json:"required"`
	Editable    bool        `json:"editable"`
	Searchable  bool        `json:"searchable"`
	Unique      bool        `json:"unique"`
	Secure      bool        `json:"secure"`
	Link        bool        `json:"link"`
	LinkModel   string      `json:"link_model"`
	Option      interface{} `json:"option"`
	Default     string      `json:"default"`
	Placeholder string      `json:"placeholder"`
	Description string      `json:"description"`
}

// UpdateAttributeReq 更新属性请求
type UpdateAttributeReq struct {
	FieldName   string      `json:"field_name"`
	FieldType   string      `json:"field_type"`
	GroupID     int64       `json:"group_id"`
	DisplayName string      `json:"display_name"`
	Display     *bool       `json:"display"`
	Index       int         `json:"index"`
	Required    *bool       `json:"required"`
	Editable    *bool       `json:"editable"`
	Searchable  *bool       `json:"searchable"`
	Unique      *bool       `json:"unique"`
	Secure      *bool       `json:"secure"`
	Link        *bool       `json:"link"`
	LinkModel   string      `json:"link_model"`
	Option      interface{} `json:"option"`
	Default     string      `json:"default"`
	Placeholder string      `json:"placeholder"`
	Description string      `json:"description"`
}

// CreateAttributeGroupReq 创建属性分组请求
type CreateAttributeGroupReq struct {
	UID         string `json:"uid" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Index       int    `json:"index"`
	Description string `json:"description"`
}

// UpdateAttributeGroupReq 更新属性分组请求
type UpdateAttributeGroupReq struct {
	Name        string `json:"name"`
	Index       int    `json:"index"`
	Description string `json:"description"`
}

// AttributeVO 属性视图对象
type AttributeVO struct {
	ID          int64       `json:"id"`
	FieldUID    string      `json:"field_uid"`
	FieldName   string      `json:"field_name"`
	FieldType   string      `json:"field_type"`
	ModelUID    string      `json:"model_uid"`
	GroupID     int64       `json:"group_id"`
	DisplayName string      `json:"display_name"`
	Display     bool        `json:"display"`
	Index       int         `json:"index"`
	Required    bool        `json:"required"`
	Editable    bool        `json:"editable"`
	Searchable  bool        `json:"searchable"`
	Unique      bool        `json:"unique"`
	Secure      bool        `json:"secure"`
	Link        bool        `json:"link"`
	LinkModel   string      `json:"link_model"`
	Option      interface{} `json:"option"`
	Default     string      `json:"default"`
	Placeholder string      `json:"placeholder"`
	Description string      `json:"description"`
	CreateTime  int64       `json:"create_time"`
	UpdateTime  int64       `json:"update_time"`
}

// AttributeGroupVO 属性分组视图对象
type AttributeGroupVO struct {
	ID          int64  `json:"id"`
	UID         string `json:"uid"`
	Name        string `json:"name"`
	ModelUID    string `json:"model_uid"`
	Index       int    `json:"index"`
	IsBuiltin   bool   `json:"is_builtin"`
	Description string `json:"description"`
	CreateTime  int64  `json:"create_time"`
	UpdateTime  int64  `json:"update_time"`
}

// AttributeGroupWithAttrsVO 带属性的分组视图对象
type AttributeGroupWithAttrsVO struct {
	AttributeGroupVO
	Attributes []AttributeVO `json:"attributes"`
}

// ========== Attribute Handlers ==========

// GetFieldTypes 获取字段类型列表
func (h *AttributeHandler) GetFieldTypes(ctx *gin.Context) {
	types := h.svc.GetFieldTypes()
	ctx.JSON(200, Result(types))
}

// CreateAttribute 创建属性
func (h *AttributeHandler) CreateAttribute(ctx *gin.Context, req CreateAttributeReq) (ginx.Result, error) {
	modelUID := ctx.Param("uid")

	attr := domain.Attribute{
		FieldUID:    req.FieldUID,
		FieldName:   req.FieldName,
		FieldType:   req.FieldType,
		ModelUID:    modelUID,
		GroupID:     req.GroupID,
		DisplayName: req.DisplayName,
		Display:     req.Display,
		Index:       req.Index,
		Required:    req.Required,
		Editable:    req.Editable,
		Searchable:  req.Searchable,
		Unique:      req.Unique,
		Secure:      req.Secure,
		Link:        req.Link,
		LinkModel:   req.LinkModel,
		Option:      req.Option,
		Default:     req.Default,
		Placeholder: req.Placeholder,
		Description: req.Description,
	}

	id, err := h.svc.CreateAttribute(ctx.Request.Context(), attr)
	if err != nil {
		if err == errs.ErrModelNotFound {
			return ErrorResult(errs.ModelNotFound), nil
		}
		if err == errs.ErrAttributeExists {
			return ErrorResultWithMsg(errs.AttributeExists, "attribute already exists"), nil
		}
		if err == errs.ErrInvalidAttributeUID || err == errs.ErrInvalidAttributeName || err == errs.ErrInvalidAttributeType {
			return ErrorResultWithMsg(errs.AttributeInvalid, err.Error()), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"id": id}), nil
}

// GetAttribute 获取属性
func (h *AttributeHandler) GetAttribute(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, "invalid id"))
		return
	}

	attr, err := h.svc.GetAttribute(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(404, ErrorResult(errs.AttributeNotFound))
		return
	}

	ctx.JSON(200, Result(h.toAttributeVO(attr)))
}

// ListAttributes 获取属性列表
func (h *AttributeHandler) ListAttributes(ctx *gin.Context) {
	modelUID := ctx.Param("uid")
	groupIDStr := ctx.Query("group_id")
	fieldType := ctx.Query("field_type")
	offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "100"))

	filter := domain.AttributeFilter{
		ModelUID:  modelUID,
		FieldType: fieldType,
		Offset:    offset,
		Limit:     limit,
	}

	if groupIDStr != "" {
		groupID, _ := strconv.ParseInt(groupIDStr, 10, 64)
		filter.GroupID = groupID
	}

	attrs, total, err := h.svc.ListAttributes(ctx.Request.Context(), filter)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, "list attributes failed: "+err.Error()))
		return
	}

	attrVOs := make([]AttributeVO, 0, len(attrs))
	for _, attr := range attrs {
		attrVOs = append(attrVOs, h.toAttributeVO(attr))
	}

	ctx.JSON(200, Result(map[string]any{
		"attributes": attrVOs,
		"total":      total,
	}))
}

// ListAttributesWithGroups 获取带分组的属性列表
func (h *AttributeHandler) ListAttributesWithGroups(ctx *gin.Context) {
	modelUID := ctx.Param("uid")
	if modelUID == "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, "模型UID不能为空"))
		return
	}

	groups, err := h.svc.ListAttributesWithGroups(ctx.Request.Context(), modelUID)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	result := make([]AttributeGroupWithAttrsVO, len(groups))
	for i, g := range groups {
		attrs := make([]AttributeVO, len(g.Attributes))
		for j, attr := range g.Attributes {
			attrs[j] = h.toAttributeVO(attr)
		}
		result[i] = AttributeGroupWithAttrsVO{
			AttributeGroupVO: h.toAttributeGroupVO(g.AttributeGroup),
			Attributes:       attrs,
		}
	}

	ctx.JSON(200, Result(result))
}

// UpdateAttribute 更新属性
func (h *AttributeHandler) UpdateAttribute(ctx *gin.Context, req UpdateAttributeReq) (ginx.Result, error) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return ErrorResultWithMsg(errs.ParamsError, "invalid id"), nil
	}

	existing, err := h.svc.GetAttribute(ctx.Request.Context(), id)
	if err != nil {
		return ErrorResult(errs.AttributeNotFound), nil
	}

	// 更新字段
	if req.FieldName != "" {
		existing.FieldName = req.FieldName
	}
	if req.FieldType != "" {
		existing.FieldType = req.FieldType
	}
	if req.GroupID > 0 {
		existing.GroupID = req.GroupID
	}
	if req.DisplayName != "" {
		existing.DisplayName = req.DisplayName
	}
	if req.Display != nil {
		existing.Display = *req.Display
	}
	if req.Index > 0 {
		existing.Index = req.Index
	}
	if req.Required != nil {
		existing.Required = *req.Required
	}
	if req.Editable != nil {
		existing.Editable = *req.Editable
	}
	if req.Searchable != nil {
		existing.Searchable = *req.Searchable
	}
	if req.Unique != nil {
		existing.Unique = *req.Unique
	}
	if req.Secure != nil {
		existing.Secure = *req.Secure
	}
	if req.Link != nil {
		existing.Link = *req.Link
	}
	if req.LinkModel != "" {
		existing.LinkModel = req.LinkModel
	}
	if req.Option != "" {
		existing.Option = req.Option
	}
	if req.Default != "" {
		existing.Default = req.Default
	}
	if req.Placeholder != "" {
		existing.Placeholder = req.Placeholder
	}
	if req.Description != "" {
		existing.Description = req.Description
	}

	err = h.svc.UpdateAttribute(ctx.Request.Context(), existing)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// DeleteAttribute 删除属性
func (h *AttributeHandler) DeleteAttribute(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, "invalid id"))
		return
	}

	err = h.svc.DeleteAttribute(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(nil))
}

// ========== Attribute Group Handlers ==========

// CreateAttributeGroup 创建属性分组
func (h *AttributeHandler) CreateAttributeGroup(ctx *gin.Context, req CreateAttributeGroupReq) (ginx.Result, error) {
	modelUID := ctx.Param("uid")

	group := domain.AttributeGroup{
		UID:         req.UID,
		Name:        req.Name,
		ModelUID:    modelUID,
		Index:       req.Index,
		Description: req.Description,
	}

	id, err := h.svc.CreateAttributeGroup(ctx.Request.Context(), group)
	if err != nil {
		if err == errs.ErrModelNotFound {
			return ErrorResult(errs.ModelNotFound), nil
		}
		if err == errs.ErrInvalidAttributeGroup {
			return ErrorResultWithMsg(errs.ParamsError, "invalid attribute group"), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"id": id}), nil
}

// GetAttributeGroup 获取属性分组
func (h *AttributeHandler) GetAttributeGroup(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, "invalid id"))
		return
	}

	group, err := h.svc.GetAttributeGroup(ctx.Request.Context(), id)
	if err != nil || group.ID == 0 {
		ctx.JSON(404, ErrorResultWithMsg(errs.AttributeNotFound, "attribute group not found"))
		return
	}

	ctx.JSON(200, Result(h.toAttributeGroupVO(group)))
}

// ListAttributeGroups 获取属性分组列表
func (h *AttributeHandler) ListAttributeGroups(ctx *gin.Context) {
	modelUID := ctx.Param("uid")

	groups, err := h.svc.ListAttributeGroups(ctx.Request.Context(), modelUID)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	groupVOs := make([]AttributeGroupVO, len(groups))
	for i, g := range groups {
		groupVOs[i] = h.toAttributeGroupVO(g)
	}

	ctx.JSON(200, Result(groupVOs))
}

// UpdateAttributeGroup 更新属性分组
func (h *AttributeHandler) UpdateAttributeGroup(ctx *gin.Context, req UpdateAttributeGroupReq) (ginx.Result, error) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return ErrorResultWithMsg(errs.ParamsError, "invalid id"), nil
	}

	existing, err := h.svc.GetAttributeGroup(ctx.Request.Context(), id)
	if err != nil || existing.ID == 0 {
		return ErrorResultWithMsg(errs.AttributeNotFound, "attribute group not found"), nil
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Index > 0 {
		existing.Index = req.Index
	}
	if req.Description != "" {
		existing.Description = req.Description
	}

	err = h.svc.UpdateAttributeGroup(ctx.Request.Context(), existing)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// DeleteAttributeGroup 删除属性分组
func (h *AttributeHandler) DeleteAttributeGroup(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, "invalid id"))
		return
	}

	err = h.svc.DeleteAttributeGroup(ctx.Request.Context(), id)
	if err != nil {
		if err == errs.ErrAttributeGroupNotFound {
			ctx.JSON(404, ErrorResultWithMsg(errs.AttributeNotFound, "attribute group not found"))
			return
		}
		if err == errs.ErrBuiltinGroupCannotDelete {
			ctx.JSON(400, ErrorResultWithMsg(errs.CannotDeleteBuiltin, "builtin group cannot be deleted"))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(nil))
}

// ========== Converters ==========

func (h *AttributeHandler) toAttributeVO(attr domain.Attribute) AttributeVO {
	var createTime, updateTime int64
	if !attr.CreateTime.IsZero() {
		createTime = attr.CreateTime.UnixMilli()
	}
	if !attr.UpdateTime.IsZero() {
		updateTime = attr.UpdateTime.UnixMilli()
	}
	return AttributeVO{
		ID:          attr.ID,
		FieldUID:    attr.FieldUID,
		FieldName:   attr.FieldName,
		FieldType:   attr.FieldType,
		ModelUID:    attr.ModelUID,
		GroupID:     attr.GroupID,
		DisplayName: attr.DisplayName,
		Display:     attr.Display,
		Index:       attr.Index,
		Required:    attr.Required,
		Editable:    attr.Editable,
		Searchable:  attr.Searchable,
		Unique:      attr.Unique,
		Secure:      attr.Secure,
		Link:        attr.Link,
		LinkModel:   attr.LinkModel,
		Option:      attr.Option,
		Default:     attr.Default,
		Placeholder: attr.Placeholder,
		Description: attr.Description,
		CreateTime:  createTime,
		UpdateTime:  updateTime,
	}
}

func (h *AttributeHandler) toAttributeGroupVO(group domain.AttributeGroup) AttributeGroupVO {
	var createTime, updateTime int64
	if !group.CreateTime.IsZero() {
		createTime = group.CreateTime.UnixMilli()
	}
	if !group.UpdateTime.IsZero() {
		updateTime = group.UpdateTime.UnixMilli()
	}
	return AttributeGroupVO{
		ID:          group.ID,
		UID:         group.UID,
		Name:        group.Name,
		ModelUID:    group.ModelUID,
		Index:       group.Index,
		IsBuiltin:   group.IsBuiltin,
		Description: group.Description,
		CreateTime:  createTime,
		UpdateTime:  updateTime,
	}
}
