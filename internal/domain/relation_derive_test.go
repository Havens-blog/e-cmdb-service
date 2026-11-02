package domain

import "testing"

// TestDeriveEcsDisk 锁定「从云盘子对象侧推导 ecs→disk」的行为。
//
// 背景（实测 2026-11-02）：ecs 侧无磁盘引用字段，disk 侧有 instance_id，
// 1312 条 disk 的 instance_id 精确命中 ecs 的 asset_id，708 台 ecs 全部可关联。
func TestDeriveEcsDisk(t *testing.T) {
	ecs := []Instance{
		{ID: 100, AssetID: "i-a", ModelUID: "ecs"},
		{ID: 101, AssetID: "i-b", ModelUID: "ecs"},
	}
	disk := func(id int64, asset, ref string) Instance {
		attrs := map[string]interface{}{}
		if ref != "" {
			attrs["instance_id"] = ref
		}
		return Instance{ID: id, AssetID: asset, ModelUID: "disk", Attributes: attrs}
	}

	t.Run("正常命中并给出正确方向", func(t *testing.T) {
		got, st := DeriveEcsDisk(ecs, []Instance{
			disk(200, "d-1", "i-a"),
			disk(201, "d-2", "i-b"),
		}, "ecs_disk")

		if len(got) != 2 {
			t.Fatalf("期望 2 条候选，实际 %d", len(got))
		}
		for _, c := range got {
			if c.SourceInstanceID != 100 && c.SourceInstanceID != 101 {
				t.Fatalf("源应为 ecs 实例 id，实际 %d", c.SourceInstanceID)
			}
			if c.TargetInstanceID != 200 && c.TargetInstanceID != 201 {
				t.Fatalf("目标应为 disk 实例 id，实际 %d", c.TargetInstanceID)
			}
			if c.RelationTypeUID != "ecs_disk" {
				t.Fatalf("关系类型错误: %s", c.RelationTypeUID)
			}
		}
		if st.Matched != 2 || st.Candidate != 2 {
			t.Fatalf("统计不符: %+v", st)
		}
	})

	t.Run("引用对不上目标时计入 unresolved", func(t *testing.T) {
		got, st := DeriveEcsDisk(ecs, []Instance{
			disk(200, "d-1", "i-a"),
			disk(202, "d-9", "i-not-collected"),
		}, "ecs_disk")
		if len(got) != 1 {
			t.Fatalf("期望 1 条候选，实际 %d", len(got))
		}
		if st.Unresolved != 1 {
			t.Fatalf("期望 1 条 unresolved，实际 %d", st.Unresolved)
		}
	})

	t.Run("无 instance_id 的磁盘被跳过", func(t *testing.T) {
		got, st := DeriveEcsDisk(ecs, []Instance{
			disk(203, "d-3", ""),
			disk(204, "d-4", "   "),
		}, "ecs_disk")
		if len(got) != 0 {
			t.Fatalf("空引用不应产生候选，实际 %d", len(got))
		}
		if st.Scanned != 2 {
			t.Fatalf("扫描数应为 2，实际 %d", st.Scanned)
		}
	})

	t.Run("同一对重复引用只产生一条（幂等）", func(t *testing.T) {
		got, _ := DeriveEcsDisk(ecs, []Instance{
			disk(200, "d-1", "i-a"),
			disk(200, "d-1", "i-a"), // 同 id 同 asset
		}, "ecs_disk")
		if len(got) != 1 {
			t.Fatalf("重复应去重为 1 条，实际 %d", len(got))
		}
	})

	t.Run("禁止自关联", func(t *testing.T) {
		// 构造 ecs id 与 disk id 相同的极端场景
		got, st := DeriveEcsDisk(
			[]Instance{{ID: 300, AssetID: "i-x"}},
			[]Instance{disk(300, "d-x", "i-x")},
			"ecs_disk",
		)
		if len(got) != 0 {
			t.Fatalf("自关联必须被拒，实际 %d 条", len(got))
		}
		if st.SkippedSelf != 1 {
			t.Fatalf("期望 SkippedSelf=1，实际 %d", st.SkippedSelf)
		}
	})

	t.Run("跨租户数据不参与（输入已按租户过滤）", func(t *testing.T) {
		// 本函数不含租户逻辑，租户由 repository List 保证；
		// 这里仅锁定「无匹配即无候选」的行为
		got, st := DeriveEcsDisk(
			[]Instance{{ID: 100, AssetID: "i-a"}},
			[]Instance{disk(200, "d-1", "i-other-tenant")},
			"ecs_disk",
		)
		if len(got) != 0 || st.Unresolved != 1 {
			t.Fatalf("异租户引用应 unresolved，实际 got=%d st=%+v", len(got), st)
		}
	})

	t.Run("空输入不 panic", func(t *testing.T) {
		got, st := DeriveEcsDisk(nil, nil, "ecs_disk")
		if len(got) != 0 || st.Candidate != 0 {
			t.Fatalf("空输入应返回空结果，实际 %d", len(got))
		}
	})
}
