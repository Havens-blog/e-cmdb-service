package domain

import "testing"

// TestDiffAttributes 锁定字段变更 diff 行为（与 e-cam 的 TrackChanges 同构）。
func TestDiffAttributes(t *testing.T) {
	t.Run("新增字段记 OldValue 为空", func(t *testing.T) {
		got := DiffAttributes(nil, map[string]interface{}{"cpu": 4})
		if len(got) != 1 {
			t.Fatalf("期望 1 条变更，实际 %d", len(got))
		}
		if got[0].FieldName != "cpu" || got[0].OldValue != "" || got[0].NewValue == "" {
			t.Fatalf("新增字段的 old 应为空: %+v", got[0])
		}
	})

	t.Run("删除字段记 NewValue 为空", func(t *testing.T) {
		got := DiffAttributes(map[string]interface{}{"cpu": 4}, nil)
		if len(got) != 1 {
			t.Fatalf("期望 1 条变更，实际 %d", len(got))
		}
		if got[0].FieldName != "cpu" || got[0].NewValue != "" || got[0].OldValue == "" {
			t.Fatalf("删除字段的 new 应为空: %+v", got[0])
		}
	})

	t.Run("值变化才记，不变不记", func(t *testing.T) {
		got := DiffAttributes(
			map[string]interface{}{"cpu": 4, "status": "running", "name": "a"},
			map[string]interface{}{"cpu": 8, "status": "running", "name": "a"},
		)
		if len(got) != 1 || got[0].FieldName != "cpu" {
			t.Fatalf("只 cpu 变化应记 1 条，实际 %+v", got)
		}
	})

	t.Run("忽略同步时间戳字段", func(t *testing.T) {
		got := DiffAttributes(
			map[string]interface{}{"sync_time": 1, "update_time": 2, "utime": 3},
			map[string]interface{}{"sync_time": 9, "update_time": 9, "utime": 9},
		)
		if len(got) != 0 {
			t.Fatalf("同步时间戳字段应被忽略，实际 %+v", got)
		}
	})

	t.Run("类型变化（数字 vs 字符串）记一条", func(t *testing.T) {
		got := DiffAttributes(
			map[string]interface{}{"cpu": 4},
			map[string]interface{}{"cpu": "4"},
		)
		if len(got) != 1 {
			t.Fatalf("类型变化应记一条，实际 %d", len(got))
		}
	})
}
