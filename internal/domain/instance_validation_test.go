package domain

import (
	"strings"
	"testing"
)

// TestValidateAttributes 锁定字段校验行为。
//
// 背景（2026-11-02）：后端此前完全不校验模型字段，任何无效数据都能写入。
// 本次按产品确认的规则实现：必填新建硬拦 / 更新不拦；枚举与类型仅警告。
func TestValidateAttributes(t *testing.T) {
	defs := []Attribute{
		{FieldUID: "instance_id", FieldType: "string", Required: true},
		{FieldUID: "status", FieldType: "enum", Required: true, Option: `["Running","Stopped"]`},
		{FieldUID: "cpu", FieldType: "int"},
		{FieldUID: "public_ip", FieldType: "string"},
	}

	t.Run("必填缺失且 blockRequired=true 时为错误", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"status": "Running",
		}, defs, true)
		if !res.HasErrors() {
			t.Fatalf("期望有错误，实际没有；warnings=%v", res.Warnings)
		}
		if !strings.Contains(res.Error(), "instance_id") {
			t.Fatalf("错误消息应包含缺失字段名，实际: %s", res.Error())
		}
	})

	t.Run("必填缺失且 blockRequired=false 时降级为警告（不阻断更新）", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"status": "Running",
		}, defs, false)
		if res.HasErrors() {
			t.Fatalf("更新场景不应产生错误，实际: %s", res.Error())
		}
		if len(res.Warnings) == 0 {
			t.Fatal("更新场景应产生必填缺失警告")
		}
	})

	t.Run("空字符串视为缺失", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"instance_id": "   ",
			"status":      "Running",
		}, defs, true)
		if !res.HasErrors() {
			t.Fatal("空白字符串应视为未填写")
		}
	})

	t.Run("枚举大小写不敏感：running 匹配 Running", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"instance_id": "i-1",
			"status":      "running",
		}, defs, true)
		if len(res.Warnings) != 0 {
			t.Fatalf("大小写不同不应告警，实际: %v", res.Warnings)
		}
	})

	t.Run("枚举越界仅警告不阻断", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"instance_id": "i-1",
			"status":      "unknown_value",
		}, defs, true)
		if res.HasErrors() {
			t.Fatalf("枚举越界不应阻断写入，实际错误: %s", res.Error())
		}
		if len(res.Warnings) != 1 || res.Warnings[0].Type != "enum" {
			t.Fatalf("期望 1 条 enum 警告，实际: %v", res.Warnings)
		}
	})

	t.Run("数值类型不符仅警告", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"instance_id": "i-1",
			"status":      "Running",
			"cpu":         "not-a-number",
		}, defs, true)
		if res.HasErrors() {
			t.Fatalf("类型问题不应阻断，实际: %s", res.Error())
		}
		found := false
		for _, w := range res.Warnings {
			if w.Field == "cpu" && w.Type == "type" {
				found = true
			}
		}
		if !found {
			t.Fatalf("期望 cpu 类型警告，实际: %v", res.Warnings)
		}
	})

	t.Run("数值字符串可接受（表单提交常为字符串）", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"instance_id": "i-1",
			"status":      "Running",
			"cpu":         "16",
		}, defs, true)
		for _, w := range res.Warnings {
			if w.Field == "cpu" {
				t.Fatalf("数字字符串不应告警，实际: %v", w)
			}
		}
	})

	t.Run("模型未定义的属性不参与校验", func(t *testing.T) {
		res := ValidateAttributes(map[string]interface{}{
			"instance_id": "i-1",
			"status":      "Running",
			"totally_new": "whatever",
		}, defs, true)
		if res.HasErrors() {
			t.Fatalf("自定义属性不应导致错误: %s", res.Error())
		}
	})

	t.Run("Option 为数组形态同样可解析", func(t *testing.T) {
		arrDefs := []Attribute{
			{FieldUID: "s", FieldType: "enum", Option: []interface{}{"Running", "Stopped"}},
		}
		res := ValidateAttributes(map[string]interface{}{"s": "RUNNING"}, arrDefs, true)
		if len(res.Warnings) != 0 {
			t.Fatalf("数组形态 Option 应能识别大小写，实际: %v", res.Warnings)
		}
	})

	t.Run("Option 为空时不判定枚举", func(t *testing.T) {
		emptyDefs := []Attribute{{FieldUID: "s", FieldType: "enum"}}
		res := ValidateAttributes(map[string]interface{}{"s": "anything"}, emptyDefs, true)
		if len(res.Warnings) != 0 {
			t.Fatalf("未定义可选值不应告警，实际: %v", res.Warnings)
		}
	})
}
