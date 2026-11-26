package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// fakeGroupRepo 只实现 normalizeModelGroup 相关路径需要的方法，其余方法返回零值。
type fakeGroupRepo struct {
	existing domain.ModelGroup
	created  domain.ModelGroup
	updated  domain.ModelGroup
	createN  int
}

func (f *fakeGroupRepo) Create(_ context.Context, group domain.ModelGroup) (int64, error) {
	f.createN++
	f.created = group
	return 99, nil
}

func (f *fakeGroupRepo) Update(_ context.Context, group domain.ModelGroup) error {
	f.updated = group
	return nil
}

func (f *fakeGroupRepo) Delete(context.Context, string) error { return nil }

func (f *fakeGroupRepo) GetByUID(context.Context, string) (domain.ModelGroup, error) {
	return f.existing, nil
}

func (f *fakeGroupRepo) GetByID(context.Context, int64) (domain.ModelGroup, error) {
	return domain.ModelGroup{}, nil
}

func (f *fakeGroupRepo) List(context.Context, domain.ModelGroupFilter) ([]domain.ModelGroup, error) {
	return nil, nil
}

func (f *fakeGroupRepo) Count(context.Context, domain.ModelGroupFilter) (int64, error) { return 0, nil }

func (f *fakeGroupRepo) InitBuiltinGroups(context.Context) error { return nil }

// TestModelGroupCreate_RejectsBlankFields 覆盖 Web 层先 TrimSpace 之后的兜底：
// gin 的 binding:"required" 放行 " "，Web 层裁剪后变成 ""，若服务层不判空就会写入
// 一个「看不见名字」的分组（比写入一个空格更难排查）。
func TestModelGroupCreate_RejectsBlankFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		group domain.ModelGroup
		want  error
	}{
		{
			name:  "uid 为单个空格",
			group: domain.ModelGroup{UID: "   ", Name: "计算"},
			want:  errs.ErrInvalidModelGroupUID,
		},
		{
			name:  "uid 为空串",
			group: domain.ModelGroup{UID: "", Name: "计算"},
			want:  errs.ErrInvalidModelGroupUID,
		},
		{
			name:  "name 为单个空格",
			group: domain.ModelGroup{UID: "zz_blank", Name: " "},
			want:  errs.ErrInvalidModelGroupName,
		},
		{
			name:  "name 为制表符与换行",
			group: domain.ModelGroup{UID: "zz_blank", Name: "\t\n"},
			want:  errs.ErrInvalidModelGroupName,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeGroupRepo{}
			svc := NewModelGroupService(repo, nil)

			_, err := svc.Create(context.Background(), tc.group)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Create() error = %v, want %v", err, tc.want)
			}
			if repo.createN != 0 {
				t.Fatalf("非法分组不应落库，实际 Create 被调用 %d 次", repo.createN)
			}
		})
	}
}

// TestModelGroupCreate_TrimsBeforePersist 确认合法输入会被裁剪后再落库。
func TestModelGroupCreate_TrimsBeforePersist(t *testing.T) {
	t.Parallel()

	repo := &fakeGroupRepo{}
	svc := NewModelGroupService(repo, nil)

	id, err := svc.Create(context.Background(), domain.ModelGroup{
		UID:         "  zz_trim  ",
		Name:        "  计算  ",
		Icon:        "  IconHost  ",
		Description: "  描述  ",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if id != 99 {
		t.Fatalf("Create() id = %d, want 99", id)
	}
	if repo.created.UID != "zz_trim" || repo.created.Name != "计算" {
		t.Fatalf("created = %+v, want trimmed uid/name", repo.created)
	}
	if repo.created.Icon != "IconHost" || repo.created.Description != "描述" {
		t.Fatalf("created = %+v, want trimmed icon/description", repo.created)
	}
}

// TestModelGroupUpdate_RejectsBlankName 确认更新路径同样拒绝纯空白名称。
func TestModelGroupUpdate_RejectsBlankName(t *testing.T) {
	t.Parallel()

	repo := &fakeGroupRepo{existing: domain.ModelGroup{ID: 7, UID: "zz_trim"}}
	svc := NewModelGroupService(repo, nil)

	err := svc.Update(context.Background(), domain.ModelGroup{UID: "zz_trim", Name: "   "})
	if !errors.Is(err, errs.ErrInvalidModelGroupName) {
		t.Fatalf("Update() error = %v, want %v", err, errs.ErrInvalidModelGroupName)
	}
	if repo.updated.UID != "" {
		t.Fatalf("非法更新不应落库，实际写入 %+v", repo.updated)
	}
}
