package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestDocsSpecRequiredMatchesBinding 锁定 docs/openapi.json 中 *Req 的 required 列表。
//
// 背景：cmd/openapi 只把带 binding:"required" 的字段判为必填（见 main.go 的
// required 分支），而 docs/openapi.json 是**手工维护**的产物（additionalProperties、
// 中文 description 等生成器不产出），因此「改了 binding 忘记改规范」不会被
// compareSpecs 发现 —— 它只比对路径数量、schema 名与 properties 字段名。
//
// 本次改动（CreateWebhookReq.EventTypes 去掉 binding:"required"，空列表校验下移到
// service.validateEventTypes）就是这种情况：字段在规范里从必填变可选，但语义上
// 「至少传一个事件类型」仍是硬约束。这里用测试把规范与实现对齐，重新生成规范若
// 再次漂移，CI 会红。
func TestDocsSpecRequiredMatchesBinding(t *testing.T) {
	specPath := filepath.Join("..", "..", "docs", "openapi.json")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", specPath, err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `json:"required"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("解析 %s 失败：%v", specPath, err)
	}

	cases := []struct {
		schema   string
		required []string
	}{
		// 事件类型在规范里可选：后端在 service 层强制「至少一个」，
		// 传空数组得到 400 中文原因，而不是绑定阶段的空体 400。
		{schema: "CreateWebhookReq", required: []string{"name", "target_url"}},
		{schema: "CreateTokenReq", required: []string{"name"}},
	}
	for _, c := range cases {
		got, ok := doc.Components.Schemas[c.schema]
		if !ok {
			t.Fatalf("规范里缺少 schema %s", c.schema)
		}
		want := append([]string(nil), c.required...)
		actual := append([]string(nil), got.Required...)
		sort.Strings(want)
		sort.Strings(actual)
		if len(want) != len(actual) {
			t.Fatalf("%s.required 不一致：want %v, got %v", c.schema, want, actual)
		}
		for i := range want {
			if want[i] != actual[i] {
				t.Fatalf("%s.required 不一致：want %v, got %v", c.schema, want, actual)
			}
		}
	}
}
