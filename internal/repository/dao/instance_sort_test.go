package dao

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestBuildSort 锁定实例列表排序行为。
//
// 背景（实测 2026-11-01）：实例列表此前完全不支持排序 ——
// 前端无列头排序 UI，后端 DAO 硬编码 SetSort(ctime:-1)，
// 用户只能按创建时间倒序查看。本次补上白名单排序，
// 本测试确保：
//  1. 未指定 / 非法字段时**回退默认排序**（不因参数异常而破坏既有行为）
//  2. 白名单字段可按指定方向排序
//  3. 任意用户输入都无法进入排序键（防注入、防内存排序扫全表）
func TestBuildSort(t *testing.T) {
	desc := func(t *testing.T, got bson.M, field string) {
		t.Helper()
		v, ok := got[field]
		if !ok {
			t.Fatalf("sort key %q absent, got %v", field, got)
		}
		if v != int32(-1) {
			t.Fatalf("%s direction = %v, want -1", field, v)
		}
		if len(got) != 1 {
			t.Fatalf("sort should have exactly 1 key, got %v", got)
		}
	}

	t.Run("未指定 sort_by 时回退默认 ctime 倒序", func(t *testing.T) {
		desc(t, buildSort(InstanceFilter{}), "ctime")
	})

	t.Run("白名单字段升序", func(t *testing.T) {
		got := buildSort(InstanceFilter{SortBy: "asset_name", SortDesc: false})
		if got["asset_name"] != int32(1) {
			t.Fatalf("asset_name = %v, want 1", got["asset_name"])
		}
	})

	t.Run("白名单字段降序", func(t *testing.T) {
		got := buildSort(InstanceFilter{SortBy: "asset_name", SortDesc: true})
		if got["asset_name"] != int32(-1) {
			t.Fatalf("asset_name = %v, want -1", got["asset_name"])
		}
	})

	t.Run("动态属性字段可排序", func(t *testing.T) {
		got := buildSort(InstanceFilter{SortBy: "attributes.status", SortDesc: true})
		if got["attributes.status"] != int32(-1) {
			t.Fatalf("attributes.status = %v, want -1", got["attributes.status"])
		}
	})

	t.Run("白名单外字段必须回退默认，不得进入 sort key", func(t *testing.T) {
		for _, bad := range []string{
			"attributes.password",
			"$where",
			"ctime; rm -rf /",
			"__proto__",
			"attributes.", // 悬空前缀
			"constructor",
		} {
			got := buildSort(InstanceFilter{SortBy: bad, SortDesc: true})
			if _, leaked := got[bad]; leaked {
				t.Fatalf("非法字段 %q 进入了排序键: %v", bad, got)
			}
			// 必须回退到默认
			if got["ctime"] != int32(-1) {
				t.Fatalf("非法字段 %q 未回退默认排序: %v", bad, got)
			}
		}
	})
}
