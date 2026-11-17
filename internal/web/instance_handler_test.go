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

// TestApplyInstanceUpdate_KeepHistoricalSemantics 锁定历史字段语义不被顺手改掉：
// 字符串字段空值代表「本次不改」，attributes 为逐键合并而非整体替换。
// 若将来要支持「清空字段」，那是产品决策，需要连带改这里和前端提示。
func TestApplyInstanceUpdate_KeepHistoricalSemantics(t *testing.T) {
	inst := domain.Instance{
		AssetName:  "old-name",
		Source:     "manual",
		Owner:      "alice",
		AccountID:  2,
		Attributes: map[string]interface{}{"hostname": "h1", "ip_address": "10.0.0.1"},
	}

	applyInstanceUpdate(&inst, UpdateInstanceReq{
		AssetName:  "",
		Source:     "",
		Owner:      "",
		Attributes: map[string]interface{}{"ip_address": "10.0.0.2"},
	})

	if inst.AssetName != "old-name" {
		t.Fatalf("AssetName = %q, want old-name（空串不改）", inst.AssetName)
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
	applyInstanceUpdate(&inst, UpdateInstanceReq{AssetName: "new", Source: "import", Owner: "bob"})

	if inst.AssetName != "new" || inst.Source != "import" || inst.Owner != "bob" {
		t.Fatalf("got %q/%q/%q, want new/import/bob", inst.AssetName, inst.Source, inst.Owner)
	}
}
