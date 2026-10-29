// Command contractcheck 校验前端手写类型与后端 OpenAPI 规范是否一致。
//
// 用途：作为 CI 门禁，在合并前拦截"前端字段名与后端契约不一致"类缺陷
// （本仓库历史上已出现 4 次：uid/model_uid ×3、source_uid/source_model_uid）。
//
// 比对规则：
//   - 取 OpenAPI components.schemas 中与前端 interface 同名的类型
//   - 比较字段名集合；差异超过允许清单即失败
//
// 用法：
//
//	go run ./cmd/contractcheck -spec openapi.json -ts ../e-cmdb-web/src/api/types/cmdb.ts
//
// 退出码：0 = 一致；1 = 存在差异
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// 允许的差异（设计性/历史遗留），修复后应逐步清空。
// 格式："TypeName.field"
var allowed = map[string]string{
	// 响应信封：前端在 *Resp 类型里本地声明 code/msg/data 是本地包装约定，
	// 后端 OpenAPI 的响应 body 已统一描述该信封，故这些类型不做字段级要求。
	"InstanceListResp.code": "",
	"InstanceListResp.msg":  "",
	"InstanceListResp.data": "",
	"ModelListResp.code":    "",
	"ModelListResp.msg":     "",
	"ModelListResp.data":    "",
	// 租户由会话决定，后端不接受该字段；前端保留可选传参以兼容旧调用。
	"CreateInstanceRelationReq.tenant_id": "",
}

func main() {
	var (
		specPath = flag.String("spec", "openapi.json", "OpenAPI 规范文件")
		tsPath   = flag.String("ts", "", "前端类型文件")
	)
	flag.Parse()
	if *tsPath == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -ts")
		os.Exit(2)
	}

	specBytes, err := os.ReadFile(*specPath)
	check(err)
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	check(json.Unmarshal(specBytes, &spec))

	tsBytes, err := os.ReadFile(*tsPath)
	check(err)
	tsTypes := parseTS(string(tsBytes))

	var problems []string
	names := make([]string, 0, len(spec.Components.Schemas))
	for name := range spec.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	compared := 0
	for _, name := range names {
		tsFields, ok := tsTypes[name]
		if !ok {
			continue // 前端未声明该类型，跳过（只校验双方都有的）
		}
		compared++
		schema := spec.Components.Schemas[name]
		goFields := make([]string, 0, len(schema.Properties))
		for f := range schema.Properties {
			goFields = append(goFields, f)
		}
		sort.Strings(goFields)
		sort.Strings(tsFields)

		goSet := toSet(goFields)
		tsSet := toSet(tsFields)

		for _, f := range goFields {
			if !tsSet[f] {
				key := name + "." + f
				if _, ok := allowed[key]; ok {
					continue
				}
				problems = append(problems, fmt.Sprintf("%s: 后端有字段 %q，前端类型缺失", name, f))
			}
		}
		for _, f := range tsFields {
			if !goSet[f] {
				key := name + "." + f
				if _, ok := allowed[key]; ok {
					continue
				}
				problems = append(problems, fmt.Sprintf("%s: 前端有字段 %q，后端 schema 无此字段", name, f))
			}
		}
	}

	fmt.Printf("已比对 %d 个同名类型\n", compared)
	if len(problems) == 0 {
		fmt.Println("契约一致 ✅")
		return
	}
	fmt.Printf("\n发现 %d 处契约不一致：\n", len(problems))
	for _, p := range problems {
		fmt.Println("  - " + p)
	}
	os.Exit(1)
}

func toSet(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, s := range list {
		m[s] = true
	}
	return m
}

// parseTS 解析 TS interface 的字段名。
func parseTS(src string) map[string][]string {
	out := map[string][]string{}
	re := regexp.MustCompile(`(?s)export\s+interface\s+(\w+)[^{]*\{(.*?)\n\}`)
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		name := m[1]
		body := m[2]
		var fields []string
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "//") {
				continue
			}
			fm := regexp.MustCompile(`^(\w+)\??\s*[:?]`).FindStringSubmatch(trimmed)
			if len(fm) > 1 {
				fields = append(fields, fm[1])
			}
		}
		out[name] = fields
	}
	return out
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}
