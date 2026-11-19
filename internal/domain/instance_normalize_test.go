package domain

import "testing"

// TestInstanceNormalize 锁定标识字段的去空格归一化。
//
// 背景（2026-11-19）：asset_id 此前只做「非空」判断：
// POST /instances/upsert 传 asset_id: "   "（三个空格）能通过校验并成功入库，
// 生成一条界面上看起来「没有资产ID」、又搜不到的实例；同理 model_uid 带尾随
// 空格会造出一个不存在的模型 uid 从而变成孤儿实例。
func TestInstanceNormalize(t *testing.T) {
	t.Run("裁掉标识字段首尾空格", func(t *testing.T) {
		inst := Instance{
			ModelUID:  "  ecs ",
			AssetID:   "\ti-abc\n",
			AssetName: " 名称 ",
			Source:    " import ",
			Owner:     " 张三 ",
		}
		inst.Normalize()
		if inst.ModelUID != "ecs" {
			t.Errorf("ModelUID 应为 ecs，实际 %q", inst.ModelUID)
		}
		if inst.AssetID != "i-abc" {
			t.Errorf("AssetID 应为 i-abc，实际 %q", inst.AssetID)
		}
		if inst.AssetName != "名称" {
			t.Errorf("AssetName 应为 名称，实际 %q", inst.AssetName)
		}
		if inst.Source != "import" {
			t.Errorf("Source 应为 import，实际 %q", inst.Source)
		}
		if inst.Owner != "张三" {
			t.Errorf("Owner 应为 张三，实际 %q", inst.Owner)
		}
	})

	t.Run("纯空格归一后为空，Validate 拦截", func(t *testing.T) {
		inst := Instance{TenantID: 3, ModelUID: "ecs", AssetID: "   "}
		inst.Normalize()
		if err := inst.Validate(); err == nil {
			t.Fatal("纯空格 asset_id 归一后应被 Validate 拦截")
		}
	})

	t.Run("不动属性值内部空格", func(t *testing.T) {
		inst := Instance{
			ModelUID:   "ecs",
			AssetID:    "i-abc",
			Attributes: map[string]interface{}{"instance_name": " my host "},
		}
		inst.Normalize()
		if got := inst.Attributes["instance_name"]; got != " my host " {
			t.Errorf("属性值不应被改写，实际 %v", got)
		}
	})
}
