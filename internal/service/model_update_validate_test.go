package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// recordingModelRepo 记录 Update 收到的模型，用于验证「更新路径也走同一套裁剪 + 非空校验」。
type recordingModelRepo struct {
	exists  bool
	updated *domain.Model
}

func (r *recordingModelRepo) Create(context.Context, domain.Model) (int64, error) { return 0, nil }

func (r *recordingModelRepo) GetByUID(context.Context, string) (domain.Model, error) {
	return domain.Model{}, nil
}

func (r *recordingModelRepo) GetByID(context.Context, int64) (domain.Model, error) {
	return domain.Model{}, nil
}

func (r *recordingModelRepo) List(context.Context, domain.ModelFilter) ([]domain.Model, error) {
	return nil, nil
}

func (r *recordingModelRepo) Count(context.Context, domain.ModelFilter) (int64, error) {
	return 0, nil
}

func (r *recordingModelRepo) Update(_ context.Context, model domain.Model) error {
	copied := model
	r.updated = &copied
	return nil
}

func (r *recordingModelRepo) Delete(context.Context, string) error { return nil }

func (r *recordingModelRepo) Exists(context.Context, string) (bool, error) { return r.exists, nil }

// TestModelUpdate_RejectsBlankFields 覆盖 PUT /models/:uid 把名称改成纯空白的场景。
//
// 只断言名称：UpdateModelReq 不含 uid / category（见 model_handler.go），
// 服务层也只强制名称非空，历史行空 category 仍可编辑（见 TestModelUpdate_AllowsEmptyCategory）。
//
// 背景（OpenCodeReview 2026-11-22）：handler 只判断 req.Name != ""，单个空格能通过；
// 若 Update 不做与 Create 相同的校验，" " 会落库成一个看不见名字的模型卡片。
func TestModelUpdate_RejectsBlankFields(t *testing.T) {
	t.Parallel()

	base := domain.Model{UID: "host", Name: "主机", Category: "host"}

	cases := []struct {
		name string
		mut  func(*domain.Model)
		want error
	}{
		{"名称为单个空格", func(m *domain.Model) { m.Name = " " }, errs.ErrInvalidModelName},
		{"名称为制表符", func(m *domain.Model) { m.Name = "\t" }, errs.ErrInvalidModelName},
		{"名称为空串", func(m *domain.Model) { m.Name = "" }, errs.ErrInvalidModelName},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &recordingModelRepo{exists: true}
			svc := NewModelService(repo, nil)

			model := base
			tc.mut(&model)

			err := svc.Update(context.Background(), model)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Update() error = %v, want %v", err, tc.want)
			}
			if repo.updated != nil {
				t.Fatalf("非法更新不应落库，实际写入 %+v", *repo.updated)
			}
		})
	}
}

// TestModelUpdate_TrimsBeforePersist 确认合法更新会被裁剪后再落库。
func TestModelUpdate_TrimsBeforePersist(t *testing.T) {
	t.Parallel()

	repo := &recordingModelRepo{exists: true}
	svc := NewModelService(repo, nil)

	err := svc.Update(context.Background(), domain.Model{
		UID:      "  host  ",
		Name:     "  主机  ",
		Category: "  host  ",
		Icon:     "  caise-computer  ",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if repo.updated == nil {
		t.Fatal("Update() 应写入一次")
	}
	if repo.updated.UID != "host" || repo.updated.Name != "主机" || repo.updated.Category != "host" {
		t.Fatalf("updated = %+v, want trimmed uid/name/category", *repo.updated)
	}
	if repo.updated.Icon != "caise-computer" {
		t.Fatalf("icon = %q, want trimmed", repo.updated.Icon)
	}
}

// TestModelUpdate_AllowsEmptyCategory 锁定「历史行 category 为空仍可编辑」（评审 #2）。
//
// UpdateModelReq 不含 category，接口没有补写手段；ecam_model 数据自 e-cam-service 迁移而来，
// 历史行可能为空。若 Update 沿用 Model.Validate（要求 Category 非空），这些模型将永远无法修改。
func TestModelUpdate_AllowsEmptyCategory(t *testing.T) {
	t.Parallel()

	repo := &recordingModelRepo{exists: true}
	svc := NewModelService(repo, nil)

	err := svc.Update(context.Background(), domain.Model{UID: "host", Name: "主机", Category: ""})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if repo.updated == nil {
		t.Fatal("Update() 应写入一次")
	}
	if repo.updated.Category != "" {
		t.Fatalf("category = %q, want empty preserved", repo.updated.Category)
	}
}
