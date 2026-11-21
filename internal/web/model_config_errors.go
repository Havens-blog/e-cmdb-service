package web

import (
	"errors"

	"github.com/Havens-blog/e-common-go/ginx"

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
	case errors.Is(err, errs.ErrInvalidModelUID):
		return ErrorResultWithMsg(errs.ModelInvalid, "模型标识（UID）不能为空"), true
	case errors.Is(err, errs.ErrInvalidModelName):
		return ErrorResultWithMsg(errs.ModelInvalid, "模型名称不能为空"), true
	case errors.Is(err, errs.ErrInvalidModelCategory):
		return ErrorResultWithMsg(errs.ModelInvalid, "模型分类不能为空"), true
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
