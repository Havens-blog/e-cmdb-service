package web

import (
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// TestModelConfigErrorResult 覆盖「模型 / 模型分组」已知业务拒绝的翻译结果。
//
// 断言三件事：错误码不变（前端可能按码分支）、文案已经是中文、
// 且不再原样回传英文哨兵文案（"model has instances, cannot delete" 等）。
func TestModelConfigErrorResult(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{"模型不存在", errs.ErrModelNotFound, errs.ModelNotFound.Code, "模型不存在或已被删除"},
		{"模型 UID 重复", errs.ErrModelExists, errs.ModelExists.Code, "模型标识已存在"},
		{"模型仍有实例", errs.ErrModelHasInstances, errs.ModelHasInstances.Code, "仍有资产实例"},
		{"模型 UID 为空", errs.ErrInvalidModelUID, errs.ModelInvalid.Code, "模型标识（UID）不能为空"},
		{"模型名称为空", errs.ErrInvalidModelName, errs.ModelInvalid.Code, "模型名称不能为空"},
		{"模型分类为空", errs.ErrInvalidModelCategory, errs.ModelInvalid.Code, "模型分类不能为空"},
		{"分组不存在", errs.ErrModelGroupNotFound, errs.ModelGroupNotFound.Code, "模型分组不存在"},
		{"分组 UID 重复", errs.ErrModelGroupExists, errs.ModelGroupExists.Code, "分组标识已存在"},
		{"分组仍有模型", errs.ErrGroupHasModels, errs.GroupHasModels.Code, "该分组下仍有模型"},
		{"内置分组不可删", errs.ErrCannotDeleteBuiltin, errs.CannotDeleteBuiltin.Code, "内置分组不允许删除"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := modelConfigErrorResult(tc.err)
			if !ok {
				t.Fatalf("%v 应被识别为已知业务拒绝", tc.err)
			}
			if result.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", result.Code, tc.wantCode)
			}
			if !strings.Contains(result.Msg, tc.wantMsg) {
				t.Fatalf("msg = %q, 期望包含 %q", result.Msg, tc.wantMsg)
			}
			if !containsChinese(result.Msg) {
				t.Fatalf("msg = %q, 期望中文文案", result.Msg)
			}
			if result.Msg == tc.err.Error() {
				t.Fatalf("msg 不能与英文哨兵原文相同: %q", result.Msg)
			}
		})
	}
}

// TestModelConfigErrorResult_Wrapped 包装后的错误（%w）同样要能被识别，
// 服务层在部分路径上会用 fmt.Errorf 包装哨兵错误。
func TestModelConfigErrorResult_Wrapped(t *testing.T) {
	wrapped := errors.Join(errors.New("count failed"), errs.ErrModelHasInstances)
	result, ok := modelConfigErrorResult(wrapped)
	if !ok {
		t.Fatal("包装错误应被识别")
	}
	if result.Code != errs.ModelHasInstances.Code {
		t.Fatalf("code = %d, want %d", result.Code, errs.ModelHasInstances.Code)
	}
}

// TestModelConfigErrorResult_UnknownIsNotTranslated 未知错误不得被翻译，
// 调用方要保留原始错误便于排查（避免把数据库故障包装成「参数错误」）。
func TestModelConfigErrorResult_UnknownIsNotTranslated(t *testing.T) {
	if _, ok := modelConfigErrorResult(errors.New("mongo: connection refused")); ok {
		t.Fatal("未知错误不应被翻译")
	}
	if _, ok := modelConfigErrorResult(nil); ok {
		t.Fatal("nil 不应被翻译")
	}
}

func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}
