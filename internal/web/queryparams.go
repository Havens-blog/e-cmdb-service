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
