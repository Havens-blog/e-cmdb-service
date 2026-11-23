package domain

import (
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// TestModelValidate_TrimsBlank 纯空白不能通过必填校验。
//
// 实测（2026-11-21，:8002）：POST /api/v1/cmdb/models 传
// {"uid":" ","name":" ","category":" "} 时 gin 的 binding:"required" 认字符串非空，
// 请求进入服务层并通过 Validate，落库出一个没有标识、没有名称的模型卡片。
func TestModelValidate_TrimsBlank(t *testing.T) {
	cases := []struct {
		name  string
		model Model
		want  error
	}{
		{"uid 纯空格", Model{UID: "  ", Name: "名称", Category: "host"}, errs.ErrInvalidModelUID},
		{"name 纯空格", Model{UID: "zz_uid", Name: "\t", Category: "host"}, errs.ErrInvalidModelName},
		{"category 纯空格", Model{UID: "zz_uid", Name: "名称", Category: "  "}, errs.ErrInvalidModelCategory},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.model.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestModelValidate_NormalizesWhitespace 校验通过时把裁剪后的值回写到接收者，
// 保证后续唯一性检查与落库用同一个规范化 UID（否则 " zz " 与 "zz" 会被当成两个模型）。
func TestModelValidate_NormalizesWhitespace(t *testing.T) {
	m := Model{UID: "  zz_p271  ", Name: "  名称  ", Category: " host "}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if m.UID != "zz_p271" {
		t.Fatalf("UID = %q, want %q", m.UID, "zz_p271")
	}
	if m.Name != "名称" {
		t.Fatalf("Name = %q, want %q", m.Name, "名称")
	}
	if m.Category != "host" {
		t.Fatalf("Category = %q, want %q", m.Category, "host")
	}
}

// TestModelValidate_KeepsInnerSpaces 只裁剪首尾，内部空格属于名称的一部分，不能被改写。
func TestModelValidate_KeepsInnerSpaces(t *testing.T) {
	m := Model{UID: "zz_p271", Name: "云 主机", Category: "host"}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if m.Name != "云 主机" {
		t.Fatalf("Name = %q, want %q", m.Name, "云 主机")
	}
}

// TestModelNormalize_TrimsParentAndProvider 回归：ParentUID/Provider 此前不在裁剪清单里。
//
// parent_uid=" " 落库后 IsTopLevel()（ParentUID == ""）与 IsSubModel()（ParentUID != ""）
// 双双为 false，模型层级自相矛盾——正是「归一清单两处各写一份」造成的漏网字段。
func TestModelNormalize_TrimsParentAndProvider(t *testing.T) {
	m := Model{UID: " host ", Name: " 主机 ", Category: " 计算 ", Level: 1, ParentUID: "   ", Provider: " all "}

	m.Normalize()

	if m.ParentUID != "" {
		t.Fatalf("ParentUID = %q, want 空串", m.ParentUID)
	}
	if !m.IsTopLevel() {
		t.Fatal("裁剪后 parent_uid 为空，应判定为顶层模型")
	}
	if m.IsSubModel() {
		t.Fatal("裁剪后 parent_uid 为空，不应判定为子模型")
	}
	if m.Provider != "all" {
		t.Fatalf("Provider = %q, want all", m.Provider)
	}
	if m.UID != "host" || m.Name != "主机" || m.Category != "计算" {
		t.Fatalf("其余字段未按预期裁剪: %+v", m)
	}
}

// TestModelNormalize_KeepsSubModel 归一不得把真实子模型的父级清空。
func TestModelNormalize_KeepsSubModel(t *testing.T) {
	m := Model{UID: "ecs", Name: "云服务器", Category: "计算", Level: 2, ParentUID: " compute "}

	m.Normalize()

	if m.ParentUID != "compute" || !m.IsSubModel() {
		t.Fatalf("ParentUID = %q, IsSubModel = %v, want compute/true", m.ParentUID, m.IsSubModel())
	}
}
