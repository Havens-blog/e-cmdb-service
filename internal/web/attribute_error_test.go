package web

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// TestAttributeErrorResult 锁住「业务拒绝必须有中文可读原因」这条约束。
//
// 背景：服务层用英文哨兵错误表达拒绝（errs.ErrAttributeExists 等），handler 曾把
// err.Error() 原文当 msg 返回，前端弹窗只显示笼统的「操作失败」，用户无法自助修复。
func TestAttributeErrorResult(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "模型不存在",
			err:      errs.ErrModelNotFound,
			wantCode: errs.ModelNotFound.Code,
			wantMsg:  "模型不存在或已被删除",
		},
		{
			name:     "字段标识重复",
			err:      errs.ErrAttributeExists,
			wantCode: errs.AttributeExists.Code,
			wantMsg:  "字段标识已存在",
		},
		{
			name:     "字段标识为空",
			err:      errs.ErrInvalidAttributeUID,
			wantCode: errs.AttributeInvalid.Code,
			wantMsg:  "字段标识不能为空",
		},
		{
			name:     "字段名称为空",
			err:      errs.ErrInvalidAttributeName,
			wantCode: errs.AttributeInvalid.Code,
			wantMsg:  "字段名称不能为空",
		},
		{
			name:     "字段类型不支持",
			err:      errs.ErrInvalidAttributeType,
			wantCode: errs.AttributeInvalid.Code,
			wantMsg:  "字段类型不支持",
		},
		{
			name:     "字段不存在",
			err:      errs.ErrAttributeNotFound,
			wantCode: errs.AttributeNotFound.Code,
			wantMsg:  "字段不存在",
		},
		{
			name:     "字段分组无效",
			err:      errs.ErrInvalidAttributeGroup,
			wantCode: errs.ParamsError.Code,
			wantMsg:  "字段分组无效",
		},
		{
			name:     "字段分组不存在",
			err:      errs.ErrAttributeGroupNotFound,
			wantCode: errs.AttributeNotFound.Code,
			wantMsg:  "字段分组不存在",
		},
		{
			name:     "内置分组不可删",
			err:      errs.ErrBuiltinGroupCannotDelete,
			wantCode: errs.CannotDeleteBuiltin.Code,
			wantMsg:  "内置分组不允许删除",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := attributeErrorResult(tc.err)
			if !ok {
				t.Fatalf("attributeErrorResult(%v) 未命中，期望被翻译", tc.err)
			}
			if result.Code != tc.wantCode {
				t.Errorf("code = %d, want %d", result.Code, tc.wantCode)
			}
			if !strings.Contains(result.Msg, tc.wantMsg) {
				t.Errorf("msg = %q, 期望包含 %q", result.Msg, tc.wantMsg)
			}
			// 文案必须是中文可读提示，不能是英文哨兵原文。
			if result.Msg == tc.err.Error() {
				t.Errorf("msg 仍是服务层英文原文 %q", result.Msg)
			}
			if !containsChinese(result.Msg) {
				t.Errorf("msg = %q 不含中文", result.Msg)
			}
		})
	}
}

// TestAttributeErrorResult_AttributeInUse 字段仍被实例数据使用时，落到 400014，
// 且文案携带 service 算出的实例数（用户据此知道要先处理多少条数据）。
// 包装过的错误也要能命中：errors.As 会沿 Unwrap 链查找类型化错误。
func TestAttributeErrorResult_AttributeInUse(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		count string
	}{
		{name: "直接返回", err: &service.AttributeInUseError{ModelUID: "host", FieldUID: "status", Count: 3}, count: "3 个实例"},
		{name: "被包装", err: fmt.Errorf("delete attribute: %w", &service.AttributeInUseError{ModelUID: "host", FieldUID: "status", Count: 12}), count: "12 个实例"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := attributeErrorResult(tc.err)
			if !ok {
				t.Fatalf("attributeErrorResult(%v) 未命中，期望被翻译", tc.err)
			}
			if result.Code != errs.AttributeInUse.Code {
				t.Errorf("code = %d, want %d", result.Code, errs.AttributeInUse.Code)
			}
			if !strings.Contains(result.Msg, tc.count) {
				t.Errorf("msg = %q, 期望包含 %q", result.Msg, tc.count)
			}
			if !strings.Contains(result.Msg, "无法删除") && !strings.Contains(result.Msg, "删除后这些数据将不再显示") {
				t.Errorf("msg = %q, 期望说明删除后果", result.Msg)
			}
			if got := attributeErrorStatus(result); got != 400 {
				t.Errorf("字段被使用 status = %d, want 400", got)
			}
		})
	}
}

// TestAttributeErrorResult_UnknownIsNotTranslated 未知错误不能被误翻译成业务提示，
// 否则真正的服务端故障会被当成用户输入问题、前端不再重试。
func TestAttributeErrorResult_UnknownIsNotTranslated(t *testing.T) {
	if _, ok := attributeErrorResult(errors.New("mongo: connection refused")); ok {
		t.Fatal("未知错误不应被翻译成业务提示")
	}
}

// TestAttributeErrorStatus 业务拒绝的状态码：不存在回 404，其余回 400。
func TestAttributeErrorStatus(t *testing.T) {
	notFound, _ := attributeErrorResult(errs.ErrAttributeNotFound)
	if got := attributeErrorStatus(notFound); got != 404 {
		t.Errorf("字段不存在 status = %d, want 404", got)
	}
	exists, _ := attributeErrorResult(errs.ErrAttributeExists)
	if got := attributeErrorStatus(exists); got != 400 {
		t.Errorf("字段标识重复 status = %d, want 400", got)
	}
	builtin, _ := attributeErrorResult(errs.ErrBuiltinGroupCannotDelete)
	if got := attributeErrorStatus(builtin); got != 400 {
		t.Errorf("内置分组不可删 status = %d, want 400", got)
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
