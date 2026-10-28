package domain

import (
	"sort"
	"testing"
)

func model(uid, provider string) Model {
	return Model{UID: uid, Name: uid, Provider: provider}
}

func TestBuildModelAliases(t *testing.T) {
	tests := []struct {
		name   string
		models []Model
		want   map[string]string
	}{
		{
			name:   "空模型集返回空映射(非 nil)",
			models: nil,
			want:   map[string]string{},
		},
		{
			name: "通用回退:裸名模型对每个已知厂商生成别名",
			models: []Model{
				model("snapshot", "all"),
			},
			want: map[string]string{
				"aliyun_snapshot":     "snapshot",
				"aws_snapshot":        "snapshot",
				"azure_snapshot":      "snapshot",
				"tencent_snapshot":    "snapshot",
				"huawei_snapshot":     "snapshot",
				"volcengine_snapshot": "snapshot",
				"volcano_snapshot":    "snapshot",
			},
		},
		{
			name: "精确命中的真实模型不生成别名",
			models: []Model{
				model("ecs", "all"),
				model("aliyun_vpc", "aliyun"),
				model("vpc", "all"),
			},
			// aliyun_vpc 是真实模型 → 该键不生成(缺失断言见 NoAliasForRealModels);
			// 其余厂商的 *_vpc 与全部 *_ecs 走通用回退
			want: map[string]string{
				"aliyun_ecs":     "ecs",
				"aws_ecs":        "ecs",
				"azure_ecs":      "ecs",
				"tencent_ecs":    "ecs",
				"huawei_ecs":     "ecs",
				"volcengine_ecs": "ecs",
				"volcano_ecs":    "ecs",
				"aws_vpc":        "vpc",
				"azure_vpc":      "vpc",
				"tencent_vpc":    "vpc",
				"huawei_vpc":     "vpc",
				"volcengine_vpc": "vpc",
				"volcano_vpc":    "vpc",
			},
		},
		{
			name: "厂商族桥接优先于通用回退:volcengine_ecs → volcano_ecs 而非 ecs",
			models: []Model{
				model("ecs", "all"),
				model("volcano_ecs", "volcano"),
			},
			want: map[string]string{
				"aliyun_ecs":     "ecs",
				"aws_ecs":        "ecs",
				"azure_ecs":      "ecs",
				"tencent_ecs":    "ecs",
				"huawei_ecs":     "ecs",
				"volcengine_ecs": "volcano_ecs", // 桥接覆盖通用回退
				// volcano_ecs 是真实模型 → 不生成 volcano_ecs 键
			},
		},
		{
			name: "无族模型时 volcengine 走通用回退",
			models: []Model{
				model("snapshot", "all"),
				model("volcano_ecs", "volcano"),
			},
			want: map[string]string{
				"volcengine_snapshot": "snapshot",    // 无 volcano_snapshot → 通用回退
				"volcengine_ecs":      "volcano_ecs", // 有 volcano_ecs → 桥接
			},
		},
		{
			name: "无任何目标模型的类型不生成别名(vswitch 缺口不掩盖)",
			models: []Model{
				model("ecs", "all"),
			},
			want: map[string]string{
				"aliyun_ecs":     "ecs",
				"aws_ecs":        "ecs",
				"azure_ecs":      "ecs",
				"tencent_ecs":    "ecs",
				"huawei_ecs":     "ecs",
				"volcengine_ecs": "ecs",
				"volcano_ecs":    "ecs",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildModelAliases(tt.models)
			if got == nil {
				t.Fatal("返回 nil map,应为非 nil(空映射也要可序列化)")
			}
			// 逐键断言 want 中的每条映射
			for k, v := range tt.want {
				if gv, ok := got[k]; !ok {
					t.Errorf("缺少别名 %q → %q", k, v)
				} else if gv != v {
					t.Errorf("别名 %q = %q, want %q", k, gv, v)
				}
			}
		})
	}
}

// TestBuildModelAliases_NoAliasForRealModels 真实模型 uid 一律不作为别名键出现。
func TestBuildModelAliases_NoAliasForRealModels(t *testing.T) {
	models := []Model{
		model("ecs", "all"),
		model("snapshot", "all"),
		model("aliyun_vpc", "aliyun"),
		model("tencent_ecs", "tencent"),
		model("volcano_ecs", "volcano"),
	}
	got := BuildModelAliases(models)
	for _, m := range models {
		if _, ok := got[m.UID]; ok {
			t.Errorf("真实模型 %q 不应出现在别名映射中", m.UID)
		}
	}
}

// TestBuildModelAliases_Deterministic 多次调用结果一致(map 遍历序不影响结果)。
func TestBuildModelAliases_Deterministic(t *testing.T) {
	models := []Model{
		model("ecs", "all"),
		model("volcano_ecs", "volcano"),
		model("volcano_vpc", "volcano"),
		model("snapshot", "all"),
	}
	first := BuildModelAliases(models)
	for i := 0; i < 20; i++ {
		got := BuildModelAliases(models)
		if len(got) != len(first) {
			t.Fatalf("第 %d 次结果长度 %d != 首次 %d", i, len(got), len(first))
		}
		for k, v := range first {
			if gv := got[k]; gv != v {
				t.Fatalf("第 %d 次 %q = %q, 首次 = %q", i, k, gv, v)
			}
		}
	}
	// 抽样打印,便于人工核对映射形态
	keys := make([]string, 0, len(first))
	for k := range first {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("别名共 %d 条,样例: %v", len(first), keys[:min(6, len(keys))])
}
