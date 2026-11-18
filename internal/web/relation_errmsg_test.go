package web

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// TestInstanceRelationErrorResult 锁定关系创建失败时面向用户的中文文案。
//
// 修复前前端弹窗直接显示服务层的英文原文（"relation already exists"、
// "instance not found: instance_id=999999999"），对最终用户没有可读性。
func TestInstanceRelationErrorResult(t *testing.T) {
	t.Run("重复关系给出中文提示", func(t *testing.T) {
		result, ok := instanceRelationErrorResult(errs.ErrRelationExists)
		if !ok {
			t.Fatal("ErrRelationExists 应被识别为业务错误")
		}
		if result.Code != errs.ParamsError.Code {
			t.Fatalf("code = %d, want %d", result.Code, errs.ParamsError.Code)
		}
		if !strings.Contains(result.Msg, "已存在") {
			t.Fatalf("提示应说明关系已存在，实际 %q", result.Msg)
		}
	})

	t.Run("资产不存在给出中文提示（含包装错误）", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: instance_id=%d", errs.ErrInstanceNotFound, 999999999)
		result, ok := instanceRelationErrorResult(wrapped)
		if !ok {
			t.Fatal("包装后的 ErrInstanceNotFound 应被识别")
		}
		if !strings.Contains(result.Msg, "资产不存在") {
			t.Fatalf("提示应说明资产不存在，实际 %q", result.Msg)
		}
		// 不要把内部 ID 细节抛给最终用户。
		if strings.Contains(result.Msg, "999999999") {
			t.Fatalf("提示不应包含内部实例 ID，实际 %q", result.Msg)
		}
	})

	t.Run("其他错误不拦截", func(t *testing.T) {
		if _, ok := instanceRelationErrorResult(errors.New("mongo timeout")); ok {
			t.Fatal("未知错误应继续走系统错误分支")
		}
		if _, ok := instanceRelationErrorResult(nil); ok {
			t.Fatal("nil 错误不应被拦截")
		}
	})
}
