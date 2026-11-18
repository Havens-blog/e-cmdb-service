package web

import (
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// TestApplyInstanceUpdate_AccountID 锁定「编辑实例时云账号ID能真正落库」。
//
// 背景（2026-11-17 实测）：编辑弹窗里「云账号ID」可编辑，但
// UpdateInstanceReq 没有 AccountID 字段、前端也不发送，
// PUT /cmdb/instances/:id 返回 code=0「success」而账号值保持原样 ——
// 用户看到「更新成功」，数据没变。DAO 的 Update 本来就会写 account_id，
// 断点只在请求体到既有实例这一层，因此在此处补指针语义。
func TestApplyInstanceUpdate_AccountID(t *testing.T) {
	ptr := func(v int64) *int64 { return &v }

	tests := []struct {
		name      string
		initial   int64
		req       *int64
		wantAfter int64
	}{
		{name: "未传账号保持原值", initial: 5, req: nil, wantAfter: 5},
		{name: "改挂到其它账号", initial: 5, req: ptr(7), wantAfter: 7},
		{name: "显式传0解除关联", initial: 5, req: ptr(0), wantAfter: 0},
		{name: "原值0改挂到账号", initial: 0, req: ptr(9), wantAfter: 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := domain.Instance{ID: 1, TenantID: 3, AccountID: tt.initial}
			applyInstanceUpdate(&inst, UpdateInstanceReq{AccountID: tt.req})
			if inst.AccountID != tt.wantAfter {
				t.Fatalf("AccountID = %d, want %d", inst.AccountID, tt.wantAfter)
			}
		})
	}
}

// TestApplyInstanceUpdate_UnsetKeepsValues 锁定「未传的字段一律不改」。
//
// 2026-11-18 起语义改为「指针 nil=不改、""=清空」，因此本用例用 nil 表达“没传”，
// 与旧的「空串=不改」区分开——旧语义会让用户永远无法清空非必填字段。
func TestApplyInstanceUpdate_UnsetKeepsValues(t *testing.T) {
	inst := domain.Instance{
		AssetName:  "old-name",
		Source:     "manual",
		Owner:      "alice",
		AccountID:  2,
		Attributes: map[string]interface{}{"hostname": "h1", "ip_address": "10.0.0.1"},
	}

	applyInstanceUpdate(&inst, UpdateInstanceReq{
		Attributes: map[string]interface{}{"ip_address": "10.0.0.2"},
	})

	if inst.AssetName != "old-name" {
		t.Fatalf("AssetName = %q, want old-name（未传不改）", inst.AssetName)
	}
	if inst.Source != "manual" || inst.Owner != "alice" {
		t.Fatalf("Source/Owner = %q/%q, want manual/alice", inst.Source, inst.Owner)
	}
	if inst.AccountID != 2 {
		t.Fatalf("AccountID = %d, want 2（未传不改）", inst.AccountID)
	}
	if inst.Attributes["hostname"] != "h1" {
		t.Fatalf("hostname = %v, want h1（未提交的键保持原值）", inst.Attributes["hostname"])
	}
	if inst.Attributes["ip_address"] != "10.0.0.2" {
		t.Fatalf("ip_address = %v, want 10.0.0.2", inst.Attributes["ip_address"])
	}
}

// TestApplyInstanceUpdate_NonEmptyWins 非空字符串照常覆盖，防止过度收紧。
func TestApplyInstanceUpdate_NonEmptyWins(t *testing.T) {
	inst := domain.Instance{AssetName: "old", Source: "manual", Owner: "alice"}
	str := func(v string) *string { return &v }
	applyInstanceUpdate(&inst, UpdateInstanceReq{
		AssetName: str("new"), Source: str("import"), Owner: str("bob"),
	})

	if inst.AssetName != "new" || inst.Source != "import" || inst.Owner != "bob" {
		t.Fatalf("got %q/%q/%q, want new/import/bob", inst.AssetName, inst.Source, inst.Owner)
	}
}

// TestApplyInstanceUpdate_ClearOptionalField 锁定 2026-11-18 产品确认的「清空」语义：
// 显式传空串（顶层字段）或 null（attributes 的键）表示清空；
// 清空的属性键会从文档中删除，而不仅是置为 null。
func TestApplyInstanceUpdate_ClearOptionalField(t *testing.T) {
	empty := ""
	inst := domain.Instance{
		Source:     "sync",
		Owner:      "alice",
		Attributes: map[string]interface{}{"hostname": "h1", "ip_address": "10.0.0.1"},
	}

	cleared := applyInstanceUpdate(&inst, UpdateInstanceReq{
		Source:     &empty,
		Owner:      &empty,
		Attributes: map[string]interface{}{"hostname": nil},
	})

	if inst.Source != "" || inst.Owner != "" {
		t.Fatalf("Source/Owner = %q/%q, want both cleared", inst.Source, inst.Owner)
	}
	if _, ok := inst.Attributes["hostname"]; ok {
		t.Fatalf("hostname 应被删除，实际仍存在：%v", inst.Attributes["hostname"])
	}
	if inst.Attributes["ip_address"] != "10.0.0.1" {
		t.Fatalf("未提交的 ip_address 应保持原值，实际 %v", inst.Attributes["ip_address"])
	}
	if len(cleared) != 1 || cleared[0] != "hostname" {
		t.Fatalf("cleared = %v, want [hostname]", cleared)
	}
}

// TestApplyInstanceUpdate_ClearReportsSortedKeys 清空多个属性时返回稳定顺序，
// 便于上层拼错误文案与断言。
func TestApplyInstanceUpdate_ClearReportsSortedKeys(t *testing.T) {
	inst := domain.Instance{Attributes: map[string]interface{}{"z_field": "1", "a_field": "2", "m_field": "3"}}
	cleared := applyInstanceUpdate(&inst, UpdateInstanceReq{
		Attributes: map[string]interface{}{"z_field": nil, "a_field": nil},
	})
	if len(cleared) != 2 || cleared[0] != "a_field" || cleared[1] != "z_field" {
		t.Fatalf("cleared = %v, want [a_field z_field]", cleared)
	}
	if len(inst.Attributes) != 1 || inst.Attributes["m_field"] != "3" {
		t.Fatalf("剩余属性应为 {m_field:3}，实际 %v", inst.Attributes)
	}
}
