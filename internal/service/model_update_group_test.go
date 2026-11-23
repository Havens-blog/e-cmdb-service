package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"go.mongodb.org/mongo-driver/mongo"
)

// groupByIDRepo 让 GetByID 返回预设分组（不存在时 ID=0），用于校验 model_group_id。
type groupByIDRepo struct {
	fakeGroupRepo
	byID domain.ModelGroup
}

func (f *groupByIDRepo) GetByID(context.Context, int64) (domain.ModelGroup, error) {
	return f.byID, nil
}

// TestModelUpdate_ValidatesModelGroupID 锁定「model_group_id 不能指向幽灵分组」。
//
// 缺陷背景（评审指出，2026-11-23）：改成指针语义后任何非 nil 值都会直写，
// Mongo 无外键，{"model_group_id": 9999} 会被静默落库，模型随即从所有分组列表消失
// （分组视图按 model_group_id 反查模型）。0 是明确语义（归入未分组），负数应拒绝。
func TestModelUpdate_ValidatesModelGroupID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		groupID int64
		byID    domain.ModelGroup
		want    error
	}{
		{"归入未分组", 0, domain.ModelGroup{}, nil},
		{"指向存在分组", 7, domain.ModelGroup{ID: 7, UID: "basic"}, nil},
		{"负数是非法值", -1, domain.ModelGroup{}, errs.ErrModelGroupInvalid},
		{"幽灵分组被拒", 9999, domain.ModelGroup{}, errs.ErrModelGroupNotFound},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &recordingModelRepo{exists: true}
			svc := NewModelService(repo, nil, &groupByIDRepo{byID: tc.byID})

			err := svc.Update(context.Background(), domain.Model{UID: "host", Name: "主机", ModelGroupID: tc.groupID})
			if tc.want == nil {
				if err != nil {
					t.Fatalf("应允许更新: %v", err)
				}
				if repo.updated == nil {
					t.Fatal("应写入仓储")
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("应返回 %v，实际 %v", tc.want, err)
			}
			if repo.updated != nil {
				t.Fatal("被拒时不应写库")
			}
		})
	}
}

// updateErrModelRepo 让 Update 返回预设错误，锁定并发删除时的 404 语义。
type updateErrModelRepo struct {
	recordingModelRepo
	updateErr error
}

func (r *updateErrModelRepo) Update(context.Context, domain.Model) error { return r.updateErr }

func TestModelUpdate_MissingRowIsNotFound(t *testing.T) {
	t.Parallel()

	repo := &updateErrModelRepo{
		recordingModelRepo: recordingModelRepo{exists: true},
		updateErr:          mongo.ErrNoDocuments,
	}
	svc := NewModelService(repo, nil, nil)

	if err := svc.Update(context.Background(), domain.Model{UID: "host", Name: "主机"}); !errors.Is(err, errs.ErrModelNotFound) {
		t.Fatalf("应翻成 ErrModelNotFound，实际 %v", err)
	}
}
