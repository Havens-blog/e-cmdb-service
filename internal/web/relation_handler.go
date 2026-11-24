package web

import (
	"errors"
	"strconv"

	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// RelationHandler 关系HTTP处理器
type RelationHandler struct {
	modelRelSvc service.ModelRelationTypeService
	relationSvc service.RelationService
	topologySvc service.TopologyService
}

// NewRelationHandler 创建关系处理器
func NewRelationHandler(
	modelRelSvc service.ModelRelationTypeService,
	relationSvc service.RelationService,
	topologySvc service.TopologyService,
) *RelationHandler {
	return &RelationHandler{
		modelRelSvc: modelRelSvc,
		relationSvc: relationSvc,
		topologySvc: topologySvc,
	}
}

// RegisterRoutes 注册关系相关路由
func (h *RelationHandler) RegisterRoutes(r *gin.RouterGroup) {
	// 模型关系类型
	modelRelGroup := r.Group("/model-relations")
	{
		modelRelGroup.POST("", ginx.WrapBody[CreateModelRelationReq](h.CreateModelRelation))
		modelRelGroup.GET("", h.ListModelRelations)
		modelRelGroup.GET("/:uid", h.GetModelRelation)
		modelRelGroup.PUT("/:uid", ginx.WrapBody[UpdateModelRelationReq](h.UpdateModelRelation))
		modelRelGroup.DELETE("/:uid", h.DeleteModelRelation)
	}

	// 实例关系
	//
	// 本子组要求已选定租户：domain.InstanceRelation 与 InstanceRelationFilter 都带
	// TenantID，且 dao/relation.go:193 是 `if filter.TenantID != 0`——租户为 0 时
	// 谓词被丢弃，会返回全部租户的实例关系。
	// 注意上面的 /model-relations 子组**不加**守卫：ModelRelationType 不含 TenantID，
	// 模型关系类型是与租户无关的元数据。
	instRelGroup := r.Group("/instance-relations")
	instRelGroup.Use(middleware.RequireTenant(elog.DefaultLogger))
	{
		instRelGroup.POST("", ginx.WrapBody[CreateInstanceRelationReq](h.CreateInstanceRelation))
		instRelGroup.POST("/batch", ginx.WrapBody[CreateBatchInstanceRelationReq](h.CreateBatchInstanceRelation))
		// 从既有实例属性推导关系（dry_run 缺省 true，安全优先）
		instRelGroup.POST("/derive", ginx.WrapBody[DeriveInstanceRelationsReq](h.DeriveInstanceRelations))
		instRelGroup.GET("", h.ListInstanceRelations)
		instRelGroup.DELETE("/:id", h.DeleteInstanceRelation)
	}

	// 拓扑
	//
	// 本子组亦为混合，故逐路由挂守卫：
	//   /instance/:id  —— 以 GetTenantID 构造 TopologyQuery.TenantID，经 service 转成
	//                     InstanceRelationFilter 后落到 dao/relation.go:193 的 `!= 0` 谓词。
	//   /related/:id   —— service 层原本漏传租户（topology.go:242），本轮已补为显式
	//                     参数，故它现在同样消费会话租户。
	//   /model         —— **不加**守卫：只按 provider 查模型关系图，ModelRelationType
	//                     不含 TenantID，与租户无关。
	topoGroup := r.Group("/topology")
	{
		topoGroup.GET("/instance/:id",
			middleware.RequireTenant(elog.DefaultLogger),
			h.GetInstanceTopology)
		topoGroup.GET("/related/:id",
			middleware.RequireTenant(elog.DefaultLogger),
			h.GetRelatedInstances)
		topoGroup.GET("/model", h.GetModelTopology)
	}
}

// ==================== 请求/响应结构体 ====================

// CreateModelRelationReq 创建模型关系类型请求
type CreateModelRelationReq struct {
	UID            string `json:"uid" binding:"required"`
	Name           string `json:"name" binding:"required"`
	SourceModelUID string `json:"source_model_uid" binding:"required"`
	TargetModelUID string `json:"target_model_uid" binding:"required"`
	RelationType   string `json:"relation_type" binding:"required"`
	Direction      string `json:"direction"`
	SourceToTarget string `json:"source_to_target"`
	TargetToSource string `json:"target_to_source"`
	Description    string `json:"description"`
}

// UpdateModelRelationReq 更新模型关系类型请求
type UpdateModelRelationReq struct {
	Name           string `json:"name"`
	SourceToTarget string `json:"source_to_target"`
	TargetToSource string `json:"target_to_source"`
	Description    string `json:"description"`
}

// CreateInstanceRelationReq 创建实例关系请求
type CreateInstanceRelationReq struct {
	SourceInstanceID int64  `json:"source_instance_id" binding:"required"`
	TargetInstanceID int64  `json:"target_instance_id" binding:"required"`
	RelationTypeUID  string `json:"relation_type_uid" binding:"required"`
}

// CreateBatchInstanceRelationReq 批量创建实例关系请求
type CreateBatchInstanceRelationReq struct {
	Relations []CreateInstanceRelationReq `json:"relations" binding:"required"`
}

// ModelRelationVO 模型关系类型视图对象
type ModelRelationVO struct {
	ID             int64  `json:"id"`
	UID            string `json:"uid"`
	Name           string `json:"name"`
	SourceModelUID string `json:"source_model_uid"`
	TargetModelUID string `json:"target_model_uid"`
	RelationType   string `json:"relation_type"`
	Direction      string `json:"direction"`
	SourceToTarget string `json:"source_to_target"`
	TargetToSource string `json:"target_to_source"`
	Description    string `json:"description"`
	CreateTime     int64  `json:"create_time"`
	UpdateTime     int64  `json:"update_time"`
}

// InstanceRelationVO 实例关系视图对象
type InstanceRelationVO struct {
	ID               int64  `json:"id"`
	SourceInstanceID int64  `json:"source_instance_id"`
	TargetInstanceID int64  `json:"target_instance_id"`
	RelationTypeUID  string `json:"relation_type_uid"`
	TenantID         int64  `json:"tenant_id"`
	CreateTime       int64  `json:"create_time"`
	// 两端实例摘要（列表页直接展示，避免前端二次查询）
	SourceAssetID   string `json:"source_asset_id"`
	SourceAssetName string `json:"source_asset_name"`
	SourceModelUID  string `json:"source_model_uid"`
	TargetAssetID   string `json:"target_asset_id"`
	TargetAssetName string `json:"target_asset_name"`
	TargetModelUID  string `json:"target_model_uid"`
}

// ==================== 模型关系类型处理方法 ====================

func (h *RelationHandler) CreateModelRelation(ctx *gin.Context, req CreateModelRelationReq) (ginx.Result, error) {
	rel := domain.ModelRelationType{
		UID:            req.UID,
		Name:           req.Name,
		SourceModelUID: req.SourceModelUID,
		TargetModelUID: req.TargetModelUID,
		RelationType:   req.RelationType,
		Direction:      req.Direction,
		SourceToTarget: req.SourceToTarget,
		TargetToSource: req.TargetToSource,
		Description:    req.Description,
	}

	id, err := h.modelRelSvc.Create(ctx.Request.Context(), rel)
	if err != nil {
		if err == errs.ErrRelationExists {
			return ErrorResultWithMsg(errs.ParamsError,
				"该关系类型已存在：源模型、目标模型与关系类型不能重复定义"), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"id": id}), nil
}

func (h *RelationHandler) ListModelRelations(ctx *gin.Context) {
	if rejectUnsupportedQuery(ctx, modelRelationListParamSet) {
		return
	}
	sourceModelUID := ctx.Query("source_model_uid")
	targetModelUID := ctx.Query("target_model_uid")
	relationType := ctx.Query("relation_type")
	offset, limit, pagingErr := resolvePagingWithin(ctx.Query("offset"), ctx.Query("limit"), defaultListLimit, maxModelRelationListLimit)
	if pagingErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, pagingErr))
		return
	}

	filter := domain.ModelRelationTypeFilter{
		SourceModelUID: sourceModelUID,
		TargetModelUID: targetModelUID,
		RelationType:   relationType,
		Offset:         offset,
		Limit:          limit,
	}

	rels, total, err := h.modelRelSvc.List(ctx.Request.Context(), filter)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	vos := make([]ModelRelationVO, len(rels))
	for i, rel := range rels {
		vos[i] = h.toModelRelationVO(rel)
	}

	ctx.JSON(200, Result(map[string]interface{}{"relations": vos, "total": total}))
}

func (h *RelationHandler) GetModelRelation(ctx *gin.Context) {
	uid := ctx.Param("uid")

	rel, err := h.modelRelSvc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		if err == errs.ErrRelationNotFound {
			ctx.JSON(404, ErrorResult(errs.RelationNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(h.toModelRelationVO(rel)))
}

func (h *RelationHandler) UpdateModelRelation(ctx *gin.Context, req UpdateModelRelationReq) (ginx.Result, error) {
	uid := ctx.Param("uid")

	existing, err := h.modelRelSvc.GetByUID(ctx.Request.Context(), uid)
	if err != nil {
		if err == errs.ErrRelationNotFound {
			return ErrorResult(errs.RelationNotFound), nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.SourceToTarget != "" {
		existing.SourceToTarget = req.SourceToTarget
	}
	if req.TargetToSource != "" {
		existing.TargetToSource = req.TargetToSource
	}
	if req.Description != "" {
		existing.Description = req.Description
	}

	err = h.modelRelSvc.Update(ctx.Request.Context(), existing)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

func (h *RelationHandler) DeleteModelRelation(ctx *gin.Context) {
	uid := ctx.Param("uid")

	err := h.modelRelSvc.Delete(ctx.Request.Context(), uid)
	if err != nil {
		if err == errs.ErrRelationNotFound {
			ctx.JSON(404, ErrorResult(errs.RelationNotFound))
			return
		}
		// 仍被资产关系引用：400 + 中文原因（含引用条数），
		// 不能落进 500 —— 这是可预期的业务拒绝，不是服务故障。
		var inUse *service.RelationTypeInUseError
		if errors.As(err, &inUse) {
			ctx.JSON(400, ErrorResultWithMsg(errs.RelationTypeInUse, inUse.Error()))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(nil))
}

// ==================== 实例关系处理方法 ====================

// instanceRelationErrorResult 把关系创建的服务层错误翻译成用户能读懂的提示。
//
// 服务层用 errs.ErrRelationExists / errs.ErrInstanceNotFound 表达业务拒绝，
// 此前直接透传英文原文，前端弹窗里显示的是 "relation already exists"、
// "instance not found: instance_id=999999999"。这些是给最终用户看的文案，
// 统一在这里翻译，避免每个调用点各写一份。
func instanceRelationErrorResult(err error) (ginx.Result, bool) {
	switch {
	case errors.Is(err, errs.ErrRelationExists):
		return ErrorResultWithMsg(errs.ParamsError,
			"该关联关系已存在：源资产与目标资产之间不能重复创建同一关系"), true
	case errors.Is(err, errs.ErrInstanceNotFound):
		return ErrorResultWithMsg(errs.ParamsError,
			"关联的资产不存在或已被删除，请刷新后重试"), true
	case errors.Is(err, errs.ErrRelationModelMismatch):
		// 关系类型声明了固定的「源模型 → 目标模型」，两端实例模型必须匹配，
		// 否则方向与基数语义不成立（实测可用 vpc_subnet 连接两个 host 实例）。
		// 与相邻分支（统一回 ParamsError 400001）刻意不同：同函数里已存在三个
		// 「参数不合法」的拒绝理由（重复关系、端点不存在、未选租户），前端只能拿到
		// 一份笼统文案；模型对不匹配需要在弹窗里区分并高亮「源/目标资产」两个选择框，
		// 因此独占一个对外 code。代价是这组 code 的前缀不统一，属可见取舍。
		return ErrorResultWithMsg(errs.RelationModelMismatch,
			"所选关系类型要求固定的资产类型组合，当前源资产或目标资产的类型不符合，请重新选择"), true
	case errors.Is(err, errs.ErrInvalidTenantID):
		// 会话未选择租户时 tenant_id=0，DAO 在 tenantID<=0 时不加租户谓词，
		// 直接放行会变成跨租户读取 + 无归属写入。
		// 本路由已挂 middleware.RequireTenant，正常请求到不了这里；此分支保护的是
		// 不走中间件的直接 service 调用方（定时推导、运维脚本），因此是前瞻性映射而非补洞。
		return ErrorResultWithMsg(errs.ParamsError,
			"未选择租户，无法操作实例关系；请先切换到具体租户后重试"), true
	case errors.Is(err, errs.ErrRelationNotFound):
		// 与 /model-relations 子组共用同一 sentinel（ErrRelationNotFound → 404003）：
		// 两者语义都是「关系类型（元数据）不存在」，两套 code 只会让前端多一条分支。
		//
		// 注意 sentinel 是复用的：RelationService.GetByID / Delete 在**某条实例关系**
		// 不存在时也返回 ErrRelationNotFound。那两条路径当前各自就地映射成
		// 「实例关系不存在或已被删除」，与这里的「关系类型不存在」文案不同。
		// 若将来把 GetByID/Delete 也接到本函数，必须先在 service 层把两种 not-found
		// 拆开，否则「某条关系已被删除」会被译成「关系类型不存在」，404 语义错位。
		return ErrorResultWithMsg(errs.RelationNotFound,
			"关系类型不存在或已被删除，请刷新后重新选择"), true
	}
	return ginx.Result{}, false
}

func (h *RelationHandler) CreateInstanceRelation(ctx *gin.Context, req CreateInstanceRelationReq) (ginx.Result, error) {
	rel := domain.InstanceRelation{
		SourceInstanceID: req.SourceInstanceID,
		TargetInstanceID: req.TargetInstanceID,
		RelationTypeUID:  req.RelationTypeUID,
		TenantID:         middleware.GetTenantID(ctx),
	}

	id, err := h.relationSvc.Create(ctx.Request.Context(), rel)
	if err != nil {
		if result, ok := instanceRelationErrorResult(err); ok {
			return result, nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"id": id}), nil
}

// DeriveInstanceRelationsReq 推导实例关系请求
//
// DryRun 缺省为 true（安全优先）：不显式传 false 时只统计不写入，
// 便于先核对影响面再决定是否落库。
type DeriveInstanceRelationsReq struct {
	// Rule 推导规则，当前仅支持 ecs_disk；留空等价于 ecs_disk
	Rule string `json:"rule"`
	// DryRun true=只统计；false=实际写入
	DryRun *bool `json:"dry_run"`
}

// DeriveInstanceRelations 从既有实例属性推导实例关系。
//
// 背景（实测 2026-11-02）：库中有 28 种关系类型定义，但实例关系为 0 条，
// 导致拓扑图、影响分析、依赖分析全部是空壳。
// ecs 侧没有磁盘引用字段，但 disk 侧 `instance_id` 有 1312 条精确命中
// ecs 的 asset_id（708 台 ecs 全部可关联），故从子对象侧反向推导。
func (h *RelationHandler) DeriveInstanceRelations(ctx *gin.Context, req DeriveInstanceRelationsReq) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	if tenantID <= 0 {
		return ErrorResultWithMsg(errs.ParamsError, "未选择租户，无法推导实例关系；请先切换到具体租户后重试"), nil
	}

	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	result, err := h.relationSvc.DeriveRelations(ctx.Request.Context(), tenantID, req.Rule, dryRun)
	if err != nil {
		// 与本文件其余关系写入口共用同一套映射：推导也会因关系类型缺失/租户未选而失败，
		// 直接回 SystemError + 英文原文会让前端把可修复问题当成服务端故障。
		if mapped, ok := instanceRelationErrorResult(err); ok {
			return mapped, nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}
	return Result(result), nil
}

func (h *RelationHandler) CreateBatchInstanceRelation(ctx *gin.Context, req CreateBatchInstanceRelationReq) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	rels := make([]domain.InstanceRelation, len(req.Relations))
	for i, r := range req.Relations {
		rels[i] = domain.InstanceRelation{
			SourceInstanceID: r.SourceInstanceID,
			TargetInstanceID: r.TargetInstanceID,
			RelationTypeUID:  r.RelationTypeUID,
			TenantID:         tenantID,
		}
	}

	count, err := h.relationSvc.CreateBatch(ctx.Request.Context(), rels)
	if err != nil {
		if result, ok := instanceRelationErrorResult(err); ok {
			return result, nil
		}
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(map[string]interface{}{"count": count}), nil
}

func (h *RelationHandler) ListInstanceRelations(ctx *gin.Context) {
	// 实测（2026-11-19）：?bogus=1 返回全部 1304 条实例关系。
	// 本端点尤其危险 —— 关系删除按 id 逐个执行，误判筛选范围会放大副作用。
	if rejectUnsupportedQuery(ctx, instanceRelationListParamSet) {
		return
	}
	sourceIDStr := ctx.Query("source_instance_id")
	targetIDStr := ctx.Query("target_instance_id")
	relationTypeUID := ctx.Query("relation_type_uid")
	tenantID := middleware.GetTenantID(ctx)
	offset, limit, pagingErr := resolvePaging(ctx.Query("offset"), ctx.Query("limit"))
	if pagingErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, pagingErr))
		return
	}

	sourceID, sourceErr := parsePositiveInt64(sourceIDStr, "source_instance_id")
	if sourceErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, sourceErr))
		return
	}
	targetID, targetErr := parsePositiveInt64(targetIDStr, "target_instance_id")
	if targetErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, targetErr))
		return
	}

	filter := domain.InstanceRelationFilter{
		SourceInstanceID: sourceID,
		TargetInstanceID: targetID,
		RelationTypeUID:  relationTypeUID,
		TenantID:         tenantID,
		Offset:           int64(offset),
		Limit:            int64(limit),
	}

	rels, total, err := h.relationSvc.ListDetailed(ctx.Request.Context(), filter)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	vos := make([]InstanceRelationVO, len(rels))
	for i, rel := range rels {
		vos[i] = InstanceRelationVO{
			ID:               rel.ID,
			SourceInstanceID: rel.SourceInstanceID,
			TargetInstanceID: rel.TargetInstanceID,
			RelationTypeUID:  rel.RelationTypeUID,
			TenantID:         rel.TenantID,
			CreateTime:       rel.CreateTime.UnixMilli(),
			SourceAssetID:    rel.Source.AssetID,
			SourceAssetName:  rel.Source.AssetName,
			SourceModelUID:   rel.Source.ModelUID,
			TargetAssetID:    rel.Target.AssetID,
			TargetAssetName:  rel.Target.AssetName,
			TargetModelUID:   rel.Target.ModelUID,
		}
	}

	ctx.JSON(200, Result(map[string]interface{}{"relations": vos, "total": total}))
}

func (h *RelationHandler) DeleteInstanceRelation(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	err = h.relationSvc.Delete(ctx.Request.Context(), middleware.GetTenantID(ctx), id)
	if err != nil {
		if err == errs.ErrRelationNotFound {
			ctx.JSON(404, ErrorResult(errs.RelationNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(nil))
}

// ==================== 拓扑处理方法 ====================

func (h *RelationHandler) GetInstanceTopology(ctx *gin.Context) {
	if rejectUnsupportedQuery(ctx, instanceTopologyParamSet) {
		return
	}
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	// depth 与 direction 此前都直接取值：depth 解析失败得 0，而 service 的
	// `query.Depth > 0` 判断把 0 当「不限深度」（internal/service/topology.go:82），
	// 于是错值把遍历放大成整个连通分量；direction 不在白名单则返回只有根节点的空图。
	depth, depthErr := parseBoundedInt(ctx.Query("depth"), "depth", 1, 1, maxGraphDepth)
	if depthErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, depthErr))
		return
	}
	direction, directionErr := parseTopologyDirection(ctx.Query("direction"))
	if directionErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, directionErr))
		return
	}
	modelUID := ctx.Query("model_uid")
	tenantID := middleware.GetTenantID(ctx)

	query := domain.TopologyQuery{
		InstanceID: id,
		ModelUID:   modelUID,
		TenantID:   tenantID,
		Depth:      depth,
		Direction:  direction,
	}

	graph, err := h.topologySvc.GetInstanceTopology(ctx.Request.Context(), query)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(graph))
}

func (h *RelationHandler) GetModelTopology(ctx *gin.Context) {
	if rejectUnsupportedQuery(ctx, modelTopologyParamSet) {
		return
	}
	provider := ctx.Query("provider")

	graph, err := h.topologySvc.GetModelTopology(ctx.Request.Context(), provider)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	ctx.JSON(200, Result(graph))
}

func (h *RelationHandler) GetRelatedInstances(ctx *gin.Context) {
	if rejectUnsupportedQuery(ctx, relatedInstancesParamSet) {
		return
	}
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	relationTypeUID := ctx.Query("relation_type_uid")
	tenantID := middleware.GetTenantID(ctx)

	instances, err := h.topologySvc.GetRelatedInstances(ctx.Request.Context(), tenantID, id, relationTypeUID)
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}

	vos := make([]InstanceVO, len(instances))
	for i, inst := range instances {
		vos[i] = InstanceVO{
			ID:         inst.ID,
			ModelUID:   inst.ModelUID,
			AssetID:    inst.AssetID,
			AssetName:  inst.AssetName,
			TenantID:   inst.TenantID,
			AccountID:  inst.AccountID,
			Attributes: inst.Attributes,
			CreateTime: inst.CreateTime.UnixMilli(),
			UpdateTime: inst.UpdateTime.UnixMilli(),
		}
	}

	ctx.JSON(200, Result(map[string]interface{}{"instances": vos, "total": len(vos)}))
}

func (h *RelationHandler) toModelRelationVO(rel domain.ModelRelationType) ModelRelationVO {
	return ModelRelationVO{
		ID:             rel.ID,
		UID:            rel.UID,
		Name:           rel.Name,
		SourceModelUID: rel.SourceModelUID,
		TargetModelUID: rel.TargetModelUID,
		RelationType:   rel.RelationType,
		Direction:      rel.Direction,
		SourceToTarget: rel.SourceToTarget,
		TargetToSource: rel.TargetToSource,
		Description:    rel.Description,
		CreateTime:     rel.CreateTime.UnixMilli(),
		UpdateTime:     rel.UpdateTime.UnixMilli(),
	}
}
