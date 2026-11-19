package web

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
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
		g.POST("/batch-delete", ginx.WrapBody[BatchDeleteInstanceReq](h.BatchDelete))
	}

	// /instance-facets 与 /instances 同级：gin 不允许 /instances/:id 的同级静态段，
	// 因此筛选可选值单独成路径，避免启动期路由冲突 panic。
	f := r.Group("/instance-facets")
	f.Use(middleware.RequireTenant(elog.DefaultLogger))
	f.GET("", h.Facets)
}

// Facets 返回实例列表的筛选可选值（云平台 / 区域 / 来源及总数）。
//
// 前端据此生成筛选下拉，取代「硬编码云平台 + 从当前页 20 条提取区域」的旧做法；
// 聚合失败时前端回退到静态取值，不影响列表本身。
func (h *InstanceHandler) Facets(ctx *gin.Context) {
	facets, err := h.svc.Facets(ctx.Request.Context(), middleware.GetTenantID(ctx))
	if err != nil {
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}
	ctx.JSON(200, Result(facets))
}

// CreateInstanceReq 创建实例请求
type CreateInstanceReq struct {
	ModelUID   string                 `json:"model_uid" binding:"required"`
	AssetID    string                 `json:"asset_id" binding:"required"`
	AssetName  string                 `json:"asset_name"`
	AccountID  int64                  `json:"account_id"`
	Attributes map[string]interface{} `json:"attributes"`
	Source     string                 `json:"source"` // 数据来源：sync/manual/import，空=未知
	Owner      string                 `json:"owner"`  // 责任人，空=未分配
}

// CreateBatchInstanceReq 批量创建实例请求
type CreateBatchInstanceReq struct {
	Instances []CreateInstanceReq `json:"instances" binding:"required"`
}

// UpsertInstanceReq 更新或插入实例请求
// UpsertInstanceReq 单条导入/同步写入请求。
//
// 这里刻意不加 binding:"required"：gin 的绑定失败会直接返回 HTTP 400 且响应体为空，
// 前端拿到的是一个没有 code/msg 的 400（弹窗只能提示「接口响应格式错误」）。
// 校验统一交给 service.Validate，由它返回带中文原因的 400001 信封。
type UpsertInstanceReq struct {
	ModelUID   string                 `json:"model_uid"`
	AssetID    string                 `json:"asset_id"`
	AssetName  string                 `json:"asset_name"`
	AccountID  int64                  `json:"account_id"`
	Attributes map[string]interface{} `json:"attributes"`
	Source     string                 `json:"source"`
	Owner      string                 `json:"owner"`
}

// UpsertBatchInstanceReq 批量更新或插入实例请求
type UpsertBatchInstanceReq struct {
	// 同理不加 binding:"required"：空数组是无害 no-op，不该变成空响应的 400。
	Instances []UpsertInstanceReq `json:"instances"`
}

// UpdateInstanceReq 更新实例请求
//
// AccountID 用指针区分「未传」与「显式置 0」：
//   - 字段缺失（nil）：不改动账号归属，保持部分更新语义；
//   - 显式传 0：解除云账号关联（与 Create 的默认值 0 含义一致）；
//   - 传非 0：改挂到指定云账号。
//
// 前端编辑弹窗里「云账号ID」是可编辑项，此前该字段不在请求体里，
// 导致用户改了账号、提示「更新成功」、实际值不变（静默丢弃）。
type UpdateInstanceReq struct {
	AssetName  *string                `json:"asset_name"`
	AccountID  *int64                 `json:"account_id"`
	Attributes map[string]interface{} `json:"attributes"`
	Source     *string                `json:"source"`
	Owner      *string                `json:"owner"`
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
	Source     string                 `json:"source"`
	Owner      string                 `json:"owner"`
	CreateTime int64                  `json:"create_time"`
	UpdateTime int64                  `json:"update_time"`
}

// InstanceListResp 实例列表响应
type InstanceListResp struct {
	Instances []InstanceVO `json:"instances"`
	Total     int64        `json:"total"`
}

// instanceCreateErrorResult 把创建/批量创建的错误映射成可读的中文结果。
//
// 修复前的问题（实测 2026-11-19）：
//   - 单条重复：{"code":400001,"msg":"instance already exists"} —— 英文，且不说明哪个资产；
//   - 批量重复：{"code":500001,"msg":"bulk write exception: write errors: [E11000 duplicate key
//     error collection: ecam.ecam_instance index: tenant_id_1_model_uid_1_asset_id_1 dup key: ...]"}
//     —— 把集合名、索引名、租户与模型内部信息直接抛给调用方；
//   - 必填缺失：被当成系统错误（500001），实际是调用方输入问题。
//
// 批量失败时带上「第 N 条（asset_id=…）」，让调用方能定位并重试。
func instanceCreateErrorResult(err error) (ginx.Result, bool) {
	prefix := ""
	var batchErr *service.BatchItemError
	if errors.As(err, &batchErr) {
		prefix = fmt.Sprintf("第 %d 条（asset_id=%s）：", batchErr.Index+1, batchErr.AssetID)
		err = batchErr.Err
	}

	switch {
	case errors.Is(err, errs.ErrModelNotFound):
		// 沿用单条创建既有契约（model not found → 404001），批量入口此前是 500001
		return ErrorResultWithMsg(errs.ModelNotFound, prefix+"模型不存在，请先选择有效的模型"), true
	case errors.Is(err, errs.ErrInstanceExists):
		return ErrorResultWithMsg(errs.ParamsError, prefix+"该资产已存在：同一模型下 asset_id 不能重复"), true
	case errors.Is(err, errs.ErrInvalidAttributes):
		msg := strings.TrimPrefix(err.Error(), errs.ErrInvalidAttributes.Error()+": ")
		return ErrorResultWithMsg(errs.ParamsError, prefix+"字段校验未通过："+msg), true
	}

	// 兜底：并发/竞态下仍可能撞唯一索引，不要把 E11000 原文抛给调用方。
	if strings.Contains(err.Error(), "E11000 duplicate key") {
		return ErrorResultWithMsg(errs.ParamsError, prefix+"该资产已存在：同一模型下 asset_id 不能重复"), true
	}
	return ginx.Result{}, false
}

// instanceWriteErrorResult 把导入/同步写入失败映射成可定位的中文响应。
//
// 修复前 ReplaceUpsert 的两个入口一律 ErrorResultWithMsg(errs.SystemError, err.Error())：
//   - 状态码 500001 让「你传错了数据」看起来像服务端故障；
//   - 报文是英文哨兵原文（asset id cannot be empty）且没有行号，
//     导入 50 行时用户无法知道是哪一行、也不知道前面几行是否已经写进去。
//
// 现在：调用方数据问题 → 400001 + 中文说明（批量还带行号与已写入条数）；
// 其余（仓储/网络等）仍按系统错误原样返回，避免把运维需要的信息误译。
func instanceWriteErrorResult(err error) ginx.Result {
	// 批量错误必须先整体处理：UpsertBatchError 通过 Unwrap 暴露根因，
	// 若先走 DescribeInstanceWriteError 会被翻译成「资产ID不能为空」，
	// 把行号与「已写入 N 条」一起丢掉——那正是这次要补的信息。
	var batchErr *service.UpsertBatchError
	if errors.As(err, &batchErr) {
		code := errs.SystemError
		if service.IsInstanceWriteValidationError(err) {
			code = errs.ParamsError
		}
		return ErrorResultWithMsg(code, batchErr.Error())
	}
	if service.IsInstanceWriteValidationError(err) {
		return ErrorResultWithMsg(errs.ParamsError, service.DescribeInstanceWriteError(err))
	}
	return ErrorResultWithMsg(errs.SystemError, err.Error())
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
		Source:     req.Source,
		Owner:      req.Owner,
	}

	id, err := h.svc.Create(middleware.WithOperator(ctx.Request.Context(), middleware.GetUsername(ctx)), instance)
	if err != nil {
		if result, ok := instanceCreateErrorResult(err); ok {
			return result, nil
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
			Source:     r.Source,
			Owner:      r.Owner,
		}
	}

	count, err := h.svc.CreateBatch(middleware.WithOperator(ctx.Request.Context(), middleware.GetUsername(ctx)), instances)
	if err != nil {
		if result, ok := instanceCreateErrorResult(err); ok {
			return result, nil
		}
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
		Source:     req.Source,
		Owner:      req.Owner,
	}

	err := h.svc.Upsert(middleware.WithOperator(ctx.Request.Context(), middleware.GetUsername(ctx)), instance)
	if err != nil {
		return instanceWriteErrorResult(err), nil
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
			Source:     r.Source,
			Owner:      r.Owner,
		}
	}

	err := h.svc.UpsertBatch(middleware.WithOperator(ctx.Request.Context(), middleware.GetUsername(ctx)), instances)
	if err != nil {
		return instanceWriteErrorResult(err), nil
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
	// 白名单校验放在最前面：未知参数直接报错，而不是被静默丢弃后返回全量。
	if unknown := unsupportedInstanceListParams(ctx.Request.URL.Query()); len(unknown) > 0 {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError,
			fmt.Sprintf("unsupported query parameter(s): %s", strings.Join(unknown, ", "))))
		return
	}
	// uid：模型 uid 的兼容别名（部分页面按资产搜索时传 uid=）。
	// 与 keyword 同理 —— 白名单接受了参数，就必须让它真的生效，
	// 否则「筛选条件被接受但被静默忽略」的陷阱依旧存在。
	modelUID := resolveInstanceModelFilter(ctx.Query("model_uid"), ctx.Query("uid"))
	tenantID := middleware.GetTenantID(ctx)
	accountIDStr := ctx.Query("account_id")
	// keyword：实例列表搜索框（提示语「搜索名称/ID」）的历史叫法，
	// 作为 asset_name 的兼容别名。
	//
	// 背景（实测 2026-11-17）：外部脚本按 keyword=zz-xxx- 过滤后逐条删除，
	// 但该参数从未被读取 —— 请求被静默降级成「无关键字全量列表」，
	// 脚本于是删掉了 49 条真实实例。同一类陷阱此前已出现过一次
	//（asset_id 未被读取，误删 10 条）。宁可多识别一个别名，
	// 也不能让字面上生效的过滤条件被无声忽略。
	assetName := resolveInstanceSearchTerm(ctx.Query("asset_name"), ctx.Query("keyword"))
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

	offset, limit, pagingErr := resolvePaging(ctx.Query("offset"), ctx.Query("limit"))
	if pagingErr != "" {
		// 非法分页不再退化为全量返回（改造前 limit=0/-1/abc 会一次吐出 2 万余条）。
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, pagingErr))
		return
	}

	accountID, accountErr := parsePositiveInt64(accountIDStr, "account_id")
	if accountErr != "" {
		ctx.JSON(400, ErrorResultWithMsg(errs.ParamsError, accountErr))
		return
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
	// 其余动态属性筛选：把白名单里的属性参数逐项落到 attributes 精确匹配，
	// 避免「参数被接受但查询不生效」的第二种静默失败。
	for _, key := range instanceListAttributeParams {
		if v := ctx.Query(key); v != "" {
			attributes[key] = v
		}
	}

	filter := domain.InstanceFilter{
		ModelUID:   modelUID,
		TenantID:   tenantID,
		AccountID:  accountID,
		AssetName:  assetName,
		AssetID:    assetID,
		Attributes: attributes,
		Source:     ctx.Query("source"),
		Owner:      ctx.Query("owner"),
		SortBy:     ctx.Query("sort_by"),
		SortDesc:   ctx.Query("order") == "desc",
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

// instanceListParamSet 是 GET /cmdb/instances 接受的查询参数白名单。
//
// 背景（2026-11-17 事故）：本接口此前对未知参数完全静默忽略 —— 调用方传
// keyword=zz-xxx 过滤后再逐条删除，实际拿到的是全量列表前 N 条，误删 49 条
// 真实实例；同类陷阱 2026-11-01 因 asset_id 未被读取已出现过一次（误删 10 条）。
// 与其不断补别名，不如让「传了不支持的参数」变成显式错误：过滤条件要么生效，
// 要么立刻报错，绝不安静地退化成全量查询。
var instanceListParamSet = map[string]struct{}{
	// 分页与排序
	"offset": {}, "limit": {}, "sort_by": {}, "order": {},
	// 搜索（asset_name 为正式参数，keyword 为兼容别名）
	"asset_name": {}, "keyword": {},
	// 精确定位与归属
	"asset_id": {}, "model_uid": {}, "uid": {}, "account_id": {}, "tenant_id": {},
	// 顶层字段筛选
	"status": {}, "region": {}, "provider": {}, "source": {}, "owner": {},
	// 动态属性筛选（映射到 attributes.<key>）
	"zone": {}, "vpc_id": {}, "instance_type": {}, "os_type": {},
	"charge_type": {}, "private_ip": {}, "public_ip": {}, "project_id": {},
}

// instanceListAttributeParams 是走 attributes 精确匹配的参数名。
// 这些属性在实例文档里是嵌套字段，与 status/region/provider 同类。
var instanceListAttributeParams = []string{
	"zone", "vpc_id", "instance_type", "os_type",
	"charge_type", "private_ip", "public_ip", "project_id",
}

// unsupportedInstanceListParams 返回请求里不在白名单内的参数名（升序）。
func unsupportedInstanceListParams(query url.Values) []string {
	unknown := make([]string, 0, len(query))
	for key := range query {
		if _, ok := instanceListParamSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// resolveInstanceModelFilter 决定实例列表的模型过滤条件。
// model_uid 为准（trim 后非空即用），否则回退 uid（历史别名）。
func resolveInstanceModelFilter(modelUID, uid string) string {
	if v := strings.TrimSpace(modelUID); v != "" {
		return v
	}
	return strings.TrimSpace(uid)
}

// resolveInstanceSearchTerm 决定实例列表的搜索关键字。
//
// asset_name 是正式参数；keyword 只是兼容别名，仅在 asset_name 为空时采用。
// 两者同时出现时以 asset_name 为准，而不是当成两个独立条件 AND ——
// 这样调用方拿到的是「一个搜索框」的直觉语义，不会因为多传一个参数而意外收窄结果。
func resolveInstanceSearchTerm(assetName, keyword string) string {
	if strings.TrimSpace(assetName) != "" {
		return assetName
	}
	return strings.TrimSpace(keyword)
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

	// 显式提交空资产名称 = 清空主名称，语义上等同于「让资产无法被识别」，直接拒绝。
	// （source/owner 属可清空的辅助字段，允许置空。）
	if req.AssetName != nil && strings.TrimSpace(*req.AssetName) == "" {
		return ErrorResultWithMsg(errs.ParamsError, "资产名称不能清空"), nil
	}

	cleared := applyInstanceUpdate(&existing, req)
	// 必填字段显式置空必须拒绝：清空后实例会立刻变成「缺失必填」，且无法再从表单补回原值。
	if msg := h.rejectClearingRequired(ctx.Request.Context(), existing.ModelUID, cleared); msg != "" {
		return ErrorResultWithMsg(errs.ParamsError, msg), nil
	}

	err = h.svc.Update(middleware.WithOperator(ctx.Request.Context(), middleware.GetUsername(ctx)), tenantID, existing)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), nil
	}

	return Result(nil), nil
}

// applyInstanceUpdate 把更新请求合并进既有实例，返回被「显式清空」的属性名。
//
// 语义（2026-11-18 产品确认，取代此前「空串=不改」的历史行为）：
//   - asset_name / source / owner 用指针：nil=本次不改，""=清空（asset_name 由调用方拒绝）；
//   - attributes 是逐键合并：值为 null 表示清空该属性（从文档中删除），未提交的键保持原值；
//   - account_id 用指针：nil=不改，0=解除关联，非 0=改挂。
//
// 返回值让调用方能在落库前做「必填字段不许清空」的校验。
func applyInstanceUpdate(existing *domain.Instance, req UpdateInstanceReq) []string {
	if req.AssetName != nil {
		existing.AssetName = *req.AssetName
	}
	if req.AccountID != nil {
		existing.AccountID = *req.AccountID
	}
	if req.Source != nil {
		existing.Source = *req.Source
	}
	if req.Owner != nil {
		existing.Owner = *req.Owner
	}

	var cleared []string
	if req.Attributes != nil {
		if existing.Attributes == nil {
			existing.Attributes = make(map[string]interface{})
		}
		for k, v := range req.Attributes {
			if v == nil {
				delete(existing.Attributes, k)
				cleared = append(cleared, k)
				continue
			}
			existing.Attributes[k] = v
		}
		sort.Strings(cleared)
	}
	return cleared
}

// rejectClearingRequired 判断本次请求清空的属性里是否含必填字段，返回拒绝文案（空串=放行）。
//
// 降级策略：必填定义拉取失败时放行并记日志 —— 与既有「字段定义拉取失败不阻塞表单」一致，
// 不能让附属查询故障阻断正常的编辑操作。
func (h *InstanceHandler) rejectClearingRequired(ctx context.Context, modelUID string, cleared []string) string {
	if len(cleared) == 0 || modelUID == "" {
		return ""
	}
	required, err := h.svc.RequiredAttributeNames(ctx, modelUID)
	if err != nil {
		elog.DefaultLogger.Warn("拉取必填字段定义失败，跳过清空校验",
			elog.String("model_uid", modelUID), elog.FieldErr(err))
		return ""
	}
	if len(required) == 0 {
		return ""
	}
	var names []string
	for _, key := range cleared {
		display, ok := required[key]
		if !ok {
			continue
		}
		if display == "" {
			display = key
		}
		names = append(names, display)
	}
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("以下必填字段不能清空：%s", strings.Join(names, "、"))
}

// Delete 删除实例
func (h *InstanceHandler) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	err = h.svc.Delete(middleware.WithOperator(ctx.Request.Context(), middleware.GetUsername(ctx)), middleware.GetTenantID(ctx), id)
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

// BatchDeleteInstanceReq 批量删除实例请求。
type BatchDeleteInstanceReq struct {
	IDs []int64 `json:"ids" binding:"required,min=1"`
}

// BatchDelete 批量删除实例（逐条结果，不回滚）。
func (h *InstanceHandler) BatchDelete(ctx *gin.Context, req BatchDeleteInstanceReq) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	res, err := h.svc.BatchDelete(middleware.WithOperator(ctx.Request.Context(), middleware.GetUsername(ctx)), tenantID, req.IDs)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}
	return Result(res), nil
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
		Source:     instance.Source,
		Owner:      instance.Owner,
		CreateTime: instance.CreateTime.UnixMilli(),
		UpdateTime: instance.UpdateTime.UnixMilli(),
	}
}
