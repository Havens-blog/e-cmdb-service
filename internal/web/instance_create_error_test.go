package web

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// TestInstanceCreateErrorResult_Messages 锁定用户可见文案。
//
// 修复前实测（2026-11-19）：
//   - 单条重复：{"code":400001,"msg":"instance already exists"}
//   - 批量重复：{"code":500001,"msg":"bulk write exception: ... E11000 duplicate key error
//     collection: ecam.ecam_instance index: tenant_id_1_model_uid_1_asset_id_1 ..."}
//   - 批量缺必填：{"code":0} 且落库（连错误都没有）
//
// 这些文案会直接经 ElMessage.error(error.message) 展示给用户，因此必须中文、
// 不泄漏集合名/索引名，并能定位到批次里具体哪一条。
func TestInstanceCreateErrorResult_Messages(t *testing.T) {
	batchErr := &service.BatchItemError{
		Index:    2,
		ModelUID: "ecs",
		AssetID:  "zz-batch-item",
		Err:      fmt.Errorf("%w: 缺少必填字段 provider", errs.ErrInvalidAttributes),
	}

	cases := []struct {
		name        string
		err         error
		wantCode    int
		wantParts   []string
		wantNotHave []string
	}{
		{
			name:      "实例已存在",
			err:       errs.ErrInstanceExists,
			wantCode:  errs.ParamsError.Code,
			wantParts: []string{"该资产已存在", "asset_id 不能重复"},
		},
		{
			name:      "模型不存在",
			err:       errs.ErrModelNotFound,
			wantCode:  errs.ModelNotFound.Code,
			wantParts: []string{"模型不存在"},
		},
		{
			name:      "必填字段缺失",
			err:       fmt.Errorf("%w: 缺少必填字段 provider；status", errs.ErrInvalidAttributes),
			wantCode:  errs.ParamsError.Code,
			wantParts: []string{"字段校验未通过", "provider", "status"},
			wantNotHave: []string{
				"invalid attributes",
			},
		},
		{
			name:      "批量条目错误带定位前缀",
			err:       batchErr,
			wantCode:  errs.ParamsError.Code,
			wantParts: []string{"第 3 条（asset_id=zz-batch-item）", "字段校验未通过", "provider"},
		},
		{
			name:      "并发撞唯一索引也不泄漏内部结构",
			err:       errors.New(`bulk write exception: write errors: [E11000 duplicate key error collection: ecam.ecam_instance index: tenant_id_1_model_uid_1_asset_id_1 dup key: { tenant_id: 3 }]`),
			wantCode:  errs.ParamsError.Code,
			wantParts: []string{"该资产已存在"},
			wantNotHave: []string{
				"E11000", "ecam_instance", "tenant_id_1_model_uid_1_asset_id_1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := instanceCreateErrorResult(tc.err)
			if !ok {
				t.Fatalf("应映射为业务错误，实际未命中：%v", tc.err)
			}
			if result.Code != tc.wantCode {
				t.Fatalf("code 期望 %d，实际 %d", tc.wantCode, result.Code)
			}
			msg := result.Msg
			for _, part := range tc.wantParts {
				if !strings.Contains(msg, part) {
					t.Fatalf("文案缺少 %q，实际：%s", part, msg)
				}
			}
			for _, part := range tc.wantNotHave {
				if strings.Contains(msg, part) {
					t.Fatalf("文案不应包含 %q，实际：%s", part, msg)
				}
			}
		})
	}
}

// TestInstanceCreateErrorResult_PassThrough 未识别的错误保持原样交回上层，
// 由调用方决定是否降级为系统错误。
func TestInstanceCreateErrorResult_PassThrough(t *testing.T) {
	if _, ok := instanceCreateErrorResult(errors.New("mongo: connection refused")); ok {
		t.Fatal("未识别的错误不应被拦截成参数错误")
	}
}
