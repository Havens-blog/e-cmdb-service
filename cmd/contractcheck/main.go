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
		specPath  = flag.String("spec", "openapi.json", "OpenAPI 规范文件")
		specPath2 = flag.String("spec2", "", "第二份规范：指定时改为比对两份规范是否一致（用于校验生成物未腐化）")
		tsPath    = flag.String("ts", "", "前端类型文件（-spec2 未指定时必填）")
	)
	flag.Parse()

	// 模式一：两份规范一致性（CI 门禁：docs/openapi.json 是否与源码同步）
	if *specPath2 != "" {
		compareSpecs(*specPath, *specPath2)
		return
	}

	if *tsPath == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -ts，或使用 -spec2 比对两份规范")
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
	if compared == 0 {
		// 解析不到任何同名类型说明输入有误（编码/路径/格式），
		// 此时静默通过等于门禁失效，必须失败并提示排查。
		fmt.Fprintln(os.Stderr, "错误：未比对到任何同名类型，请检查 -spec / -ts 路径与文件编码")
		os.Exit(2)
	}
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

// compareSpecs 比对两份 OpenAPI 规范的路径与 schema 结构是否一致。
// 用于 CI 门禁：确认仓库内提交的 docs/openapi.json 未与源码生成结果脱节。
func compareSpecs(pathA, pathB string) {
	type schema struct {
		Properties map[string]any `json:"properties"`
	}
	type doc struct {
		Paths      map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]schema `json:"schemas"`
		} `json:"components"`
	}
	load := func(p string) doc {
		b, err := os.ReadFile(p)
		check(err)
		var d doc
		check(json.Unmarshal(b, &d))
		return d
	}
	a, b := load(pathA), load(pathB)

	var diffs []string

	// 路径集合
	if len(a.Paths) != len(b.Paths) {
		diffs = append(diffs, fmt.Sprintf("路径数量不一致：%d vs %d", len(a.Paths), len(b.Paths)))
	}
	for p := range a.Paths {
		if _, ok := b.Paths[p]; !ok {
			diffs = append(diffs, "仅存在于 "+pathA+"："+p)
		}
	}
	for p := range b.Paths {
		if _, ok := a.Paths[p]; !ok {
			diffs = append(diffs, "仅存在于 "+pathB+"："+p)
		}
	}

	// schema 字段集合
	for name, sa := range a.Components.Schemas {
		sb, ok := b.Components.Schemas[name]
		if !ok {
			diffs = append(diffs, "schema 仅存在于 "+pathA+"："+name)
			continue
		}
		fa := make([]string, 0, len(sa.Properties))
		for f := range sa.Properties {
			fa = append(fa, f)
		}
		fb := make([]string, 0, len(sb.Properties))
		for f := range sb.Properties {
			fb = append(fb, f)
		}
		sort.Strings(fa)
		sort.Strings(fb)
		if strings.Join(fa, ",") != strings.Join(fb, ",") {
			diffs = append(diffs, fmt.Sprintf("schema %s 字段不一致：%v vs %v", name, fa, fb))
		}
	}
	for name := range b.Components.Schemas {
		if _, ok := a.Components.Schemas[name]; !ok {
			diffs = append(diffs, "schema 仅存在于 "+pathB+"："+name)
		}
	}

	if len(diffs) == 0 {
		fmt.Printf("两份规范一致 ✅（%d 条路径，%d 个 schema）\n", len(a.Paths), len(a.Components.Schemas))
		return
	}
	fmt.Printf("发现 %d 处规范差异：\n", len(diffs))
	for _, d := range diffs {
		fmt.Println("  - " + d)
	}
	fmt.Fprintln(os.Stderr, "\n请执行: go run ./cmd/openapi -root . -o docs/openapi.json 并提交")
	os.Exit(1)
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
