package web

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// 未知查询参数守卫（2026-11-19 统一收口）。
//
// 背景：GET /cmdb/instances 曾在 2026-11-17 造成一次真实事故 —— 调用方传
// `keyword=zz-xxx` 以为在筛选，实际该参数被静默忽略、返回全量列表前 N 条，
// 随后逐条删除误删了 49 条真实实例（同类陷阱在 2026-11-01 因 `asset_id`
// 未被读取已出现过一次）。修复（PR #53）把该端点改成白名单 + 400。
//
// 但同一类陷阱在其余列表端点上仍然存在，本轮取证（2026-11-19，:8002 实测）：
//
//	GET /cmdb/models?keyword=zz-no-such-model   → 200，total=81（关键词被忽略）
//	GET /cmdb/models?uid=zz-no-such             → 200，total=81
//	GET /cmdb/models?bogus=1                    → 200，total=81
//	GET /cmdb/models/ecs/attributes?bogus=1     → 200，total=17
//	GET /cmdb/model-groups?bogus=1              → 200，total=11
//	GET /cmdb/model-relations?bogus=1           → 200，total=28
//	GET /cmdb/instance-relations?bogus=1        → 200，total=1304
//	GET /cmdb/quality/issues?bogus=1            → 200，total=0
//
// 结论：过滤条件要么生效、要么立刻报错，绝不安静地退化成全量查询。
// 本文件提供统一实现，各处理器只声明自己的参数清单。

// paramSet 是查询参数白名单。用 map 而非切片，便于 O(1) 判断。
type paramSet map[string]struct{}

// newParamSet 由参数名构造白名单（字面量写法比 map 字面量更紧凑）。
func newParamSet(names ...string) paramSet {
	set := make(paramSet, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

// unsupportedQueryParams 返回请求里不在白名单内的参数名，按字典序升序。
//
// 升序是为了让同一个错误请求每次得到完全一致的报文（便于调用方断言与日志聚合）。
// 重复键（同一参数出现多次）只报一次。
func unsupportedQueryParams(query url.Values, allowed paramSet) []string {
	unknown := make([]string, 0, len(query))
	for key := range query {
		if _, ok := allowed[key]; ok {
			continue
		}
		unknown = append(unknown, key)
	}
	sort.Strings(unknown)
	return unknown
}

// rejectUnsupportedQuery 在请求带白名单外参数时写入 400 响应并返回 true。
//
// 调用方约定：
//
//	if rejectUnsupportedQuery(ctx, xxxParamSet) {
//	    return
//	}
//
// 之所以必须放在处理器最前面：一旦处理器已经开始解析其它参数，未知参数就会被
// 当作「没传」处理，用户看到的仍是全量数据。
func rejectUnsupportedQuery(ctx *gin.Context, allowed paramSet) bool {
	unknown := unsupportedQueryParams(ctx.Request.URL.Query(), allowed)
	if len(unknown) == 0 {
		return false
	}
	ctx.JSON(http.StatusBadRequest, unsupportedQueryResult(unknown))
	return true
}

// unsupportedQueryResult 构造「未知查询参数」的信封，供 (ginx.Result, error) 形态的
// 处理器直接返回（ginx.Wrap 会负责写出响应）。
func unsupportedQueryResult(unknown []string) ginx.Result {
	return ErrorResultWithMsg(errs.ParamsError,
		fmt.Sprintf("unsupported query parameter(s): %s", strings.Join(unknown, ", ")))
}

// guardQueryParams 是 rejectUnsupportedQuery 的返回值形态版本，供
// `func (h *X) Y(ctx *gin.Context) (ginx.Result, error)` 处理器使用：
//
//	if res, bad := guardQueryParams(ctx, xxxParamSet); bad {
//	    return res, nil
//	}
func guardQueryParams(ctx *gin.Context, allowed paramSet) (ginx.Result, bool) {
	unknown := unsupportedQueryParams(ctx.Request.URL.Query(), allowed)
	if len(unknown) == 0 {
		return ginx.Result{}, false
	}
	return unsupportedQueryResult(unknown), true
}

// 各读取端点的查询参数白名单。
//
// 维护规则：处理器里 `ctx.Query("x")` 读取了哪些参数，这里就必须列哪些；
// 反过来，这里列出的参数必须在处理器里真正生效（接受但不生效同样是陷阱）。
var (
	// GET /cmdb/models —— 参数与 model_handler.ListModels 的读取一致。
	modelListParamSet = newParamSet("offset", "limit",
		"provider", "category", "parent_uid", "level", "model_group_id", "extensible")

	// GET /cmdb/model-groups —— 仅分页。
	modelGroupListParamSet = newParamSet("offset", "limit")

	// GET /cmdb/models/:uid/attributes —— 与 attribute_handler.ListAttributes 一致。
	attributeListParamSet = newParamSet("offset", "limit",
		"group_id", "field_type", "required", "display", "searchable")

	// GET /cmdb/model-relations —— 与 relation_handler.ListModelRelations 一致。
	modelRelationListParamSet = newParamSet("offset", "limit",
		"source_model_uid", "target_model_uid", "relation_type")

	// GET /cmdb/instance-relations —— 与 relation_handler.ListInstanceRelations 一致。
	//
	// 注意：页面 URL 用的 instance_id / instance_role 是前端自己的 query 键，
	// 由前端换算成 source_instance_id / target_instance_id 后才发请求。
	instanceRelationListParamSet = newParamSet("offset", "limit",
		"source_instance_id", "target_instance_id", "relation_type_uid")

	// GET /cmdb/quality/issues
	qualityIssuesParamSet = newParamSet("offset", "limit", "type")

	// GET /cmdb/quality/field-issues
	qualityFieldIssuesParamSet = newParamSet("offset", "limit", "model_uid", "issue_type")

	// GET /cmdb/quality/sync-issues
	qualitySyncIssuesParamSet = newParamSet("offset", "limit", "model_uid", "issue_type", "asset_id")

	// GET /cmdb/impact
	impactParamSet = newParamSet("instance_id", "depth")

	// GET /cmdb/topology/instance/:id
	instanceTopologyParamSet = newParamSet("depth", "direction", "model_uid")

	// GET /cmdb/topology/model
	modelTopologyParamSet = newParamSet("provider")

	// GET /cmdb/topology/related/:id
	relatedInstancesParamSet = newParamSet("relation_type_uid")
)

// ---------------------------------------------------------------------------
// 参数取值校验（2026-11-19 第二轮：上一节处理「参数名不认识」，这里处理
// 「参数名认识、但取值非法」）
//
// 上节白名单解决了拼错的参数名被静默忽略。同类陷阱还有另一半：参数名在白名单
// 里，但取值非法时解析器退化成「未传」，过滤条件就此消失。实测（2026-11-19）：
//
//	GET /cmdb/models/ecs/attributes?required=zzz    → 200，total=17（= 全部）
//	GET /cmdb/models/ecs/attributes?display=zzz     → 200，total=17（= 全部）
//	GET /cmdb/models/ecs/attributes?searchable=zzz  → 200，total=17（= 全部）
//	GET /cmdb/models?extensible=zzz                 → 200，total=81（= 全部）
//	GET /cmdb/quality/issues?type=zzz               → 200，total=0（看起来「健康」）
//	GET /cmdb/quality/field-issues?issue_type=zzz   → 200，total=0
//
// 而同一批处理器的整型参数早已严格报错：
//
//	?level=zzz              → 400 level 必须是不小于 0 的整数（收到 "zzz"）
//	?source_instance_id=zzz → 400 source_instance_id 必须是大于 0 的整数（收到 "zzz"）
//
// 也就是说同一套 API 对「值非法」的处理并不一致：整型严格，布尔与枚举宽松。
// 宽松的两种后果都会骗调用方 —— 布尔退化成全量（放大副作用），枚举退化成空集
// （把「参数写错了」显示成「没有问题」）。数据质量面板尤其不能这样。
//
// 约定：空字符串（`?required=`）仍表示「未传」，保持与前端「清空筛选」一致；
// 只有非空且无法识别的值才报错。

// boolParamHint 是布尔参数的合法取值说明，报错与文档共用同一份措辞。
const boolParamHint = "true/false/1/0"

// parseOptionalBoolParam 解析可选的布尔查询参数。
//
// 返回 (nil, "") 表示未传；值非法时返回空指针与可直接回给调用方的中文原因。
// 与旧的 parseOptionalBool 的差别：旧实现把任意无法识别的值当成「未传」，
// 于是 `required=zzz` 静默变成「不按 required 过滤」并返回全部属性。
func parseOptionalBoolParam(raw, name string) (*bool, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, ""
	}
	switch strings.ToLower(value) {
	case "true", "1":
		on := true
		return &on, ""
	case "false", "0":
		off := false
		return &off, ""
	default:
		return nil, fmt.Sprintf("%s 必须是布尔值（%s，收到 %q）", name, boolParamHint, raw)
	}
}

// validateEnumParam 校验枚举型查询参数的取值，并返回**归一化后**的值。
//
// 返回 (归一化值, "") 表示放行：空字符串（含纯空格）表示「未传」，归一化为 ""；
// 命中 allowed 之一时返回去掉首尾空格的规范取值。
// 返回 ("", 中文原因) 表示拒绝，原因里列出合法取值。
//
// 两个返回值缺一不可。下游（selectIssues / selectFieldIssues / SyncIssueFilter）
// 按字符串精确比较，若这里只做校验、调用方仍把原始值透传，那么
// `?type=%20orphan_instance` 会因为校验时被 Trim 而通过、下游却匹配不到任何类型，
// 退化成 {"total":0} —— 正是本函数要消灭的「静默空集」。所以调用方必须使用
// 这里返回的归一化值，不能再用 ctx.Query 的原始串。
//
// 为什么必须报错而不是返回空集：`/quality/issues?type=zzz` 曾是
// `{"total":0}`，与「这个租户确实没有该类问题」完全无法区分。
func validateEnumParam(raw, name string, allowed []string) (string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ""
	}
	for _, ok := range allowed {
		if value == ok {
			return value, ""
		}
	}
	return "", fmt.Sprintf("%s 必须是以下值之一：%s（收到 %q）", name, strings.Join(allowed, "、"), raw)
}

// 枚举参数取值集合。
//
// qualityIssueTypes 只包含 /quality/issues 真正能返回的三类判定级问题：
// orphan_instance / missing_field 走 Mongo 下推，dangling_relation 走分析缓存。
// 字段级问题（field_type / field_enum）由 /quality/field-issues 提供，因此
// 传到这里会被拒绝并提示改用另一个端点 —— 允许它通过只会得到空集。
var (
	qualityIssueTypes = []string{
		service.QualityIssueTypeOrphan,
		service.QualityIssueTypeMissingField,
		service.QualityIssueTypeDanglingRelation,
	}

	qualityFieldIssueTypes = []string{
		service.QualityIssueTypeFieldType,
		service.QualityIssueTypeFieldEnum,
	}

	// syncIssueTypes 对应 domain.ValidateAttributes 产出的警告类型
	// （internal/domain/instance_validation.go: required / enum / type），
	// 也是 recordSyncQualityIssues 写入 IssueType 的取值。
	syncIssueTypes = []string{"required", "enum", "type"}
)
