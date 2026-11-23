package web

import (
	"errors"
	"net/http"

	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// modelConfigErrorResult 把「模型 / 模型分组」的服务层哨兵错误翻译成用户能读懂的中文。
//
// 背景（实测 2026-11-21，:8002，admin@tenant3）：
//   - 删除仍有实例的模型 host → `{"code":400009,"msg":"model has instances, cannot delete"}`
//   - 更新不存在的模型 → `{"code":404001,"msg":"model not found"}`
//   - 创建重复 UID 的模型 → `{"code":400001,"msg":"model already exists"}`
//   - 创建重名分组 → `{"code":409005,"msg":"model group already exists"}`
//
// 前端删除入口只 `ElMessage.error('删除失败')`，用户既不知道是「还有实例」还是「没权限」，
// 也无法自助恢复；同一页面里的字段（属性）侧已在同批次口径上改成了中文，两处不一致。
//
// 第二个返回值表示「是否为已知业务拒绝」：false 时调用方仍按系统错误处理（保留原始错误便于排查）。
func modelConfigErrorResult(err error) (ginx.Result, bool) {
	switch {
	case errors.Is(err, errs.ErrModelNotFound):
		return ErrorResultWithMsg(errs.ModelNotFound, "模型不存在或已被删除，请刷新后重试"), true
	case errors.Is(err, errs.ErrModelExists):
		return ErrorResultWithMsg(errs.ModelExists, "模型标识已存在：UID 不能重复，请换一个"), true
	case errors.Is(err, errs.ErrModelHasInstances):
		return ErrorResultWithMsg(errs.ModelHasInstances,
			"该模型下仍有资产实例，无法删除：请先删除或迁移这些实例后再试"), true
	// 空值类拒绝用资源专属码（ModelInvalid 400002 / ModelGroupInvalid 400006），
	// 与同批改的字段侧（attribute_handler.go 的 AttributeInvalid）保持同构：
	// 同一个包里的「参数解析失败」用 ParamsError，而「业务字段为空」各自有码，
	// 前端可据 code 定位到具体资源，msg 由这里给出中文。
	case errors.Is(err, errs.ErrInvalidModelUID):
		return ErrorResultWithMsg(errs.ModelInvalid, "模型标识（UID）不能为空"), true
	case errors.Is(err, errs.ErrInvalidModelName):
		return ErrorResultWithMsg(errs.ModelInvalid, "模型名称不能为空"), true
	case errors.Is(err, errs.ErrInvalidModelCategory):
		return ErrorResultWithMsg(errs.ModelInvalid, "模型分类不能为空"), true
	case errors.Is(err, errs.ErrInvalidModelGroupUID):
		return ErrorResultWithMsg(errs.ModelGroupInvalid, "分组标识（UID）不能为空"), true
	case errors.Is(err, errs.ErrInvalidModelGroupName):
		return ErrorResultWithMsg(errs.ModelGroupInvalid, "分组名称不能为空"), true
	case errors.Is(err, errs.ErrModelGroupInvalid):
		// 生产触发点：UpdateModelReq.model_group_id 传负数（0 表示「归入未分组」）。
		return ErrorResultWithMsg(errs.ModelGroupInvalid, "分组信息无效，请检查后重试"), true
	case errors.Is(err, errs.ErrModelGroupNotFound):
		return ErrorResultWithMsg(errs.ModelGroupNotFound, "模型分组不存在或已被删除，请刷新后重试"), true
	case errors.Is(err, errs.ErrModelGroupExists):
		return ErrorResultWithMsg(errs.ModelGroupExists, "分组标识已存在：分组 UID 不能重复，请换一个"), true
	case errors.Is(err, errs.ErrGroupHasModels):
		return ErrorResultWithMsg(errs.GroupHasModels,
			"该分组下仍有模型，无法删除：请先把这些模型移到其他分组"), true
	case errors.Is(err, errs.ErrCannotDeleteBuiltin):
		return ErrorResultWithMsg(errs.CannotDeleteBuiltin, "内置分组不允许删除"), true
	default:
		return ginx.Result{}, false
	}
}

// modelConfigErrorStatus 把「对外业务错误码」映射为 HTTP 状态码。
//
// 收敛背景：此前状态码选择散落在每个 handler 里——GetByUID 固定 404、
// Create/Update 由 ginx.WrapBody 固定 200、Delete 用 `switch result.Code` 反查，
// 同一类「业务拒绝」在 PUT 与 DELETE 上是两种 HTTP 语义。
// 更危险的是 Delete 的默认分支取 409：service 层将来新增任何哨兵错误，
// 都会被当成「冲突」返回，即使它其实是参数校验失败。
//
// 现在由这一处决定：未找到 → 404，冲突/重复 → 409，其余业务拒绝 → 400。
func modelConfigErrorStatus(code int) int {
	switch code {
	case errs.ModelNotFound.Code, errs.ModelGroupNotFound.Code:
		return http.StatusNotFound
	case errs.ModelExists.Code, errs.ModelGroupExists.Code, errs.GroupHasModels.Code, errs.ModelHasInstances.Code:
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

// writeModelConfigError 统一写出业务拒绝：HTTP 状态取 modelConfigErrorStatus，
// body 为中文信封，前端 extractApiErrorMessage 读 msg 即可展示。
func writeModelConfigError(ctx *gin.Context, err error) bool {
	result, ok := modelConfigErrorResult(err)
	if !ok {
		return false
	}
	ctx.JSON(modelConfigErrorStatus(result.Code), result)
	return true
}

// bindJSONBody 自己绑定请求体，替代 ginx.WrapBody：后者绑定失败时只写日志、
// 不写响应体，客户端拿到的是空 200，无法自助定位问题。
func bindJSONBody(ctx *gin.Context, req any) bool {
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResultWithMsg(errs.ParamsError,
			"请求体格式不正确，请检查字段类型后重试"))
		return false
	}
	return true
}
