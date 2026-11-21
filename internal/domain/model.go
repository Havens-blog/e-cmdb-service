package domain

import (
	"strings"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// Model 资源模型领域对象
type Model struct {
	ID           int64     // 业务ID
	UID          string    // 模型唯一标识
	Name         string    // 模型名称
	ModelGroupID int64     // 模型分组ID
	ParentUID    string    // 父模型UID（用于层级关系）
	Category     string    // 资源类别
	Level        int       // 层级（1=主资源，2=子资源）
	Icon         string    // 图标
	Description  string    // 描述
	Provider     string    // 云厂商（all表示通用）
	Extensible   bool      // 是否可扩展
	CreateTime   time.Time // 创建时间
	UpdateTime   time.Time // 更新时间
}

// ModelFilter 模型过滤条件
type ModelFilter struct {
	Provider     string
	Category     string
	ParentUID    string
	Level        int
	ModelGroupID int64
	Extensible   *bool
	Offset       int
	Limit        int
}

// Validate 验证模型数据。
//
// 先裁剪首尾空白再判空：gin 的 binding:"required" 只拒绝空字符串，
// 实测 uid/name/category 传单个空格可以通过校验直接落库（模型列表里出现一个没有名字的空卡片）。
// 裁剪同时回写到接收者，保证后续唯一性检查与落库使用同一个规范化后的 UID。
func (m *Model) Validate() error {
	m.UID = strings.TrimSpace(m.UID)
	m.Name = strings.TrimSpace(m.Name)
	m.Category = strings.TrimSpace(m.Category)
	if m.UID == "" {
		return errs.ErrInvalidModelUID
	}
	if m.Name == "" {
		return errs.ErrInvalidModelName
	}
	if m.Category == "" {
		return errs.ErrInvalidModelCategory
	}
	return nil
}

// IsTopLevel 是否为顶级模型
func (m *Model) IsTopLevel() bool {
	return m.Level == 1 && m.ParentUID == ""
}

// IsSubModel 是否为子模型
func (m *Model) IsSubModel() bool {
	return m.Level > 1 && m.ParentUID != ""
}
