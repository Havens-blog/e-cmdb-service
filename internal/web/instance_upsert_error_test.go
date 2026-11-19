package web

import (
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// TestInstanceWriteErrorResult 锁定导入/同步写入失败的响应契约。
//
// 背景（2026-11-19）：Upsert / UpsertBatch 两个入口一律
// ErrorResultWithMsg(errs.SystemError, err.Error())，于是「用户传了空 asset_id」
// 返回 500001（系统错误）+ 英文原文 "asset id cannot be empty"，批量时还没有
// 行号与已写入条数。导入 50 行失败时用户无法判断哪一行出错、前几行是否已入库。
func TestInstanceWriteErrorResult(t *testing.T) {
	t.Run("单条校验失败：400001 + 中文", func(t *testing.T) {
		res := instanceWriteErrorResult(errs.ErrInvalidAssetID)
		if res.Code != errs.ParamsError.Code {
			t.Errorf("code 应为 %d，实际 %d", errs.ParamsError.Code, res.Code)
		}
		if res.Msg != "资产ID不能为空" {
			t.Errorf("msg 不符：%s", res.Msg)
		}
		if strings.Contains(res.Msg, "asset id") {
			t.Errorf("不应出现英文原文：%s", res.Msg)
		}
	})

	t.Run("批量校验失败：400001 + 行号 + 已写入条数", func(t *testing.T) {
		err := &service.UpsertBatchError{Index: 1, AssetID: "zz-x", Written: 1, Total: 3, Err: errs.ErrInvalidAssetID}
		res := instanceWriteErrorResult(err)
		if res.Code != errs.ParamsError.Code {
			t.Errorf("code 应为 %d，实际 %d", errs.ParamsError.Code, res.Code)
		}
		for _, want := range []string{"第 2 条", "zz-x", "资产ID不能为空", "已写入 1 条", "共 3 条"} {
			if !strings.Contains(res.Msg, want) {
				t.Errorf("msg 缺少 %q：%s", want, res.Msg)
			}
		}
	})

	t.Run("批量仓储失败：500001 但仍带定位信息", func(t *testing.T) {
		err := &service.UpsertBatchError{Index: 0, AssetID: "zz-y", Written: 0, Total: 2, Err: errors.New("mongo: timeout")}
		res := instanceWriteErrorResult(err)
		if res.Code != errs.SystemError.Code {
			t.Errorf("运维类错误应保持系统错误码，实际 %d", res.Code)
		}
		for _, want := range []string{"第 1 条", "mongo: timeout", "已写入 0 条"} {
			if !strings.Contains(res.Msg, want) {
				t.Errorf("msg 缺少 %q：%s", want, res.Msg)
			}
		}
	})

	t.Run("未知错误原样透传", func(t *testing.T) {
		res := instanceWriteErrorResult(errors.New("boom"))
		if res.Code != errs.SystemError.Code || res.Msg != "boom" {
			t.Errorf("未知错误应原样返回，实际 code=%d msg=%s", res.Code, res.Msg)
		}
	})
}
