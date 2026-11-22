package web

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// iter272：模型关系类型创建被服务层拒绝时，必须给出中文、可操作的原因，
// 而不是英文哨兵串或被折成 500 system error。
func TestModelRelationTypeErrorResult(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "源模型不存在",
			err:      fmt.Errorf("%w: model_uid=cloud_ecs", errs.ErrRelationSourceModelNotFound),
			wantCode: errs.RelationSourceModelNotFound.Code,
			wantMsg:  "源模型不存在",
		},
		{
			name:     "目标模型不存在",
			err:      fmt.Errorf("%w: model_uid=cloud_vpc", errs.ErrRelationTargetModelNotFound),
			wantCode: errs.RelationTargetModelNotFound.Code,
			wantMsg:  "目标模型不存在",
		},
		{
			name:     "关系类型已存在",
			err:      errs.ErrRelationExists,
			wantCode: errs.RelationExists.Code,
			wantMsg:  "该关系类型已存在",
		},
		{
			name:     "UID 为空",
			err:      domain.ErrInvalidRelationTypeUID,
			wantCode: errs.RelationInvalid.Code,
			wantMsg:  "关系类型标识",
		},
		{
			name:     "名称为空",
			err:      domain.ErrInvalidRelationName,
			wantCode: errs.RelationInvalid.Code,
			wantMsg:  "关系类型名称不能为空",
		},
		{
			name:     "模型为空",
			err:      domain.ErrInvalidRelationModel,
			wantCode: errs.RelationInvalid.Code,
			wantMsg:  "源模型与目标模型都必须选择",
		},
		{
			name:     "关系类型不存在映射 404003 中文",
			err:      errs.ErrRelationNotFound,
			wantCode: errs.RelationNotFound.Code,
			wantMsg:  "关系类型不存在或已被删除",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := modelRelationTypeErrorResult(tc.err)
			if !ok {
				t.Fatalf("期望识别为业务错误，实际未翻译：%v", tc.err)
			}
			if result.Code != tc.wantCode {
				t.Fatalf("错误码不符：want %d got %d", tc.wantCode, result.Code)
			}
			msg := result.Msg
			if !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("文案缺少关键片段 %q，实际 %q", tc.wantMsg, msg)
			}
			if !hasChineseRune(msg) {
				t.Fatalf("文案应为中文，实际 %q", msg)
			}
		})
	}
}

func TestModelRelationTypeErrorResult_UnknownIsNotTranslated(t *testing.T) {
	cases := []error{
		nil,
		errors.New("mongo: connection refused"),
		errs.ErrModelNotFound,
	}
	for _, err := range cases {
		if _, ok := modelRelationTypeErrorResult(err); ok {
			t.Fatalf("未知错误不应被翻译：%v", err)
		}
	}
}

// hasChineseRune 判断字符串里是否含 CJK 统一表意文字，用来挡住把英文哨兵直接透给前端。
func hasChineseRune(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}
