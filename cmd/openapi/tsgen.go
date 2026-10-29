package main

// TS 类型生成：从 OpenAPI components.schemas 产出 TypeScript interface。
//
// 目标：让 e-cmdb-web 的 types/cmdb.ts 不再手写，从后端契约自动生成，
// 从根上消除字段名不一致（本会话已 4 次）。
//
// 命名与可选性保持与既有手写文件一致：
//   - 类型名沿用后端 struct 名（InstanceVO / CreateInstanceReq ...）
//   - 非 required 字段输出 `字段?:`
//   - 空 schema（未能推断）输出 `any`

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// tsGen 从规范 JSON 生成 TS 源码。
func tsGen(spec map[string]any) string {
	var b strings.Builder

	b.WriteString("/* eslint-disable */\n")
	b.WriteString("// 本文件由 e-cmdb-service/cmd/openapi 自动生成，请勿手工编辑。\n")
	b.WriteString("// 重新生成：go run ./cmd/openapi -root . -o docs/openapi.json -ts-out <本文件>\n\n")

	comps, _ := spec["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)

	names := make([]string, 0, len(schemas))
	for n := range schemas {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		if !isFrontendType(n) {
			continue
		}
		s, _ := schemas[n].(map[string]any)
		if s == nil {
			continue
		}
		// 只输出 web 层传输类型；domain/service 类型仅用于 $ref 解析，不进前端契约
		if layer, _ := s["x-layer"].(string); layer != "" && layer != "web" {
			continue
		}
		b.WriteString(fmt.Sprintf("export interface %s {\n", n))

		props, _ := s["properties"].(map[string]any)
		reqSet := map[string]bool{}
		// required 在内存 map 中是 []string（生成时直接构造），
		// 若经 JSON 往返则变为 []any，两种都要兼容。
		switch rl := s["required"].(type) {
		case []string:
			for _, r := range rl {
				reqSet[r] = true
			}
		case []any:
			for _, r := range rl {
				if rs, ok := r.(string); ok {
					reqSet[rs] = true
				}
			}
		}
		pnames := make([]string, 0, len(props))
		for p := range props {
			pnames = append(pnames, p)
		}
		sort.Strings(pnames)
		for _, p := range pnames {
			ps, _ := props[p].(map[string]any)
			tsType := tsTypeOf(ps)
			opt := "?"
			if reqSet[p] {
				opt = ""
			}
			// 字段名非法标识符时加引号
			field := p
			if !isIdent(p) {
				field = `"` + p + `"`
			}
			b.WriteString(fmt.Sprintf("    %s%s: %s\n", field, opt, tsType))
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// isFrontendType 判断该后端类型是否应出现在前端契约中。
// 排除：handler 结构体、service 实现体（它们只承载依赖，不是传输结构）。
func isFrontendType(name string) bool {
	if strings.HasSuffix(name, "Handler") {
		return false
	}
	// 小写开头的结构体是 service/dao 实现，非导出传输类型
	if name != "" {
		r := rune(name[0])
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

// tsTypeOf 把 JSON Schema 片段转成 TS 类型。
func tsTypeOf(s map[string]any) string {
	if s == nil || len(s) == 0 {
		return "any"
	}
	if ref, ok := s["$ref"].(string); ok {
		if i := strings.LastIndex(ref, "/"); i >= 0 {
			return ref[i+1:]
		}
		return "any"
	}
	if nullable, ok := s["nullable"].(bool); ok && nullable {
		return "any"
	}
	if ap, ok := s["additionalProperties"].(bool); ok && ap {
		return "Record<string, any>"
	}
	t, _ := s["type"].(string)
	switch t {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		items, _ := s["items"].(map[string]any)
		inner := tsTypeOf(items)
		if strings.Contains(inner, "|") {
			inner = "(" + inner + ")"
		}
		return inner + "[]"
	case "object":
		props, _ := s["properties"].(map[string]any)
		if len(props) == 0 {
			return "Record<string, any>"
		}
		// 内联对象
		var parts []string
		pn := make([]string, 0, len(props))
		for p := range props {
			pn = append(pn, p)
		}
		sort.Strings(pn)
		for _, p := range pn {
			ps, _ := props[p].(map[string]any)
			key := p
			if !isIdent(p) {
				key = `"` + p + `"`
			}
			parts = append(parts, fmt.Sprintf("%s?: %s", key, tsTypeOf(ps)))
		}
		return "{ " + strings.Join(parts, "; ") + " }"
	}
	return "any"
}

// writeTS 输出 TS 文件。
func writeTS(path string, spec map[string]any) error {
	return os.WriteFile(path, []byte(tsGen(spec)), 0o644)
}
