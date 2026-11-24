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
	t.Run("模型对不匹配回独立 code 与中文提示", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: relation_type=vpc_subnet", errs.ErrRelationModelMismatch)
		result, ok := instanceRelationErrorResult(wrapped)
		if !ok {
			t.Fatal("包装后的 ErrRelationModelMismatch 应被识别")
		}
		// 与相邻分支不同：该理由需要前端高亮两端资产选择框，因此独占一个对外 code。
		if result.Code != errs.RelationModelMismatch.Code {
			t.Fatalf("code = %d, want %d", result.Code, errs.RelationModelMismatch.Code)
		}
		if !strings.Contains(result.Msg, "资产类型") {
			t.Fatalf("提示应说明资产类型不符合，实际 %q", result.Msg)
		}
	})

	t.Run("未选择租户给出中文提示", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: tenant_id=0", errs.ErrInvalidTenantID)
		result, ok := instanceRelationErrorResult(wrapped)
		if !ok {
			t.Fatal("包装后的 ErrInvalidTenantID 应被识别")
		}
		if result.Code != errs.ParamsError.Code {
			t.Fatalf("code = %d, want %d", result.Code, errs.ParamsError.Code)
		}
		if !strings.Contains(result.Msg, "未选择租户") {
			t.Fatalf("提示应说明未选择租户，实际 %q", result.Msg)
		}
	})

	t.Run("关系类型不存在回 404 段 code", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: relation_type_uid=ecs_disk", errs.ErrRelationNotFound)
		result, ok := instanceRelationErrorResult(wrapped)
		if !ok {
			t.Fatal("包装后的 ErrRelationNotFound 应被识别")
		}
		// *NotFound 语义归 404 段，前端据此判断「资源不存在」而不是「参数不合法」；
		// 与 /model-relations 子组共用同一 code，避免同一语义分裂成两个编号。
		if result.Code != errs.RelationNotFound.Code {
			t.Fatalf("code = %d, want %d", result.Code, errs.RelationNotFound.Code)
		}
		if result.Code < 404000 || result.Code >= 405000 {
			t.Fatalf("code = %d 应落在 404 段", result.Code)
		}
		if !strings.Contains(result.Msg, "关系类型不存在") {
			t.Fatalf("提示应说明关系类型不存在，实际 %q", result.Msg)
		}
	})

}
