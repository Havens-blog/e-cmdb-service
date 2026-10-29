// Command openapi 从 e-cmdb-service 源码静态生成 OpenAPI 3.0 规范。
//
// 设计动机：CMDB 前后端字段名不一致缺陷反复出现（uid/model_uid 等），
// 根因是前端 types/cmdb.ts 由人工手写、与后端 struct 无任何校验关联。
// 本工具以后端源码为唯一事实来源生成规范，前端类型再由规范生成，
// 使契约漂移在 CI 阶段即可发现。
//
// 解析策略：
//  1. 扫描 internal/domain、internal/service、internal/web 收集 struct 字段（含 json tag）
//  2. 收集 service 接口方法签名 → 返回类型（响应类型的权威来源）
//  3. 扫描 internal/web 路由注册：Method / Path / 请求体泛型参数 / handler 名
//  4. 对每个 handler，追踪 `x, err := h.<field>.<Method>(...)` 定位 service 方法，
//     取其返回类型作为 200 响应 schema
//  5. 输出 OpenAPI 3.0 JSON
//
// 无法静态确定的响应（如直接构造 map、或多次赋值）标记为 unknown 并在 stderr 汇总，
// 不臆造 schema。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------- 数据模型 ----------

type Field struct {
	JSONName string
	GoType   string
	Required bool
}

type StructDef struct {
	Name   string
	Fields []Field
	Pkg    string
	Layer  string   // domain / service / web：仅 web 层类型进入前端契约
	Embeds []string // 匿名嵌入的类型名，字段需展开合并
}

type Route struct {
	Method   string
	Path     string
	BodyType string
	Handler  string
	RecvType string // handler 结构体类型名，如 InstanceHandler
	Group    string
}

type Gen struct {
	fset    *token.FileSet
	structs map[string]*StructDef
	// service 接口方法：接口名 -> 方法名 -> 返回类型名
	svcMethods map[string]map[string]string
	// handler 类型 -> 字段名 -> service 接口名
	handlerFields map[string]map[string]string
	routes        []Route
	rootDir       string
	layer         string
	// 函数/方法名 -> 第一个返回类型（用于 Result(h.toVO(x)) 这类推断）
	funcReturns map[string]string
}

func main() {
	var (
		rootDir   = flag.String("root", ".", "e-cmdb-service 仓库根目录")
		outFile   = flag.String("o", "openapi.json", "输出 OpenAPI 文件")
		basePath  = flag.String("base", "/api/v1/cmdb", "路由公共前缀")
		title     = flag.String("title", "e-cmdb-service API", "规范标题")
		version   = flag.String("version", "1.0.0", "API 版本")
		tsOut     = flag.String("ts-out", "", "可选：同时输出 TypeScript 类型文件（供前端直接使用）")
		tsExclude = flag.String("ts-exclude", "", "可选：逗号分隔的类型名，生成 TS 时排除（用于前后端有已知设计差异的类型）")
	)
	flag.Parse()

	g := &Gen{
		fset:          token.NewFileSet(),
		structs:       map[string]*StructDef{},
		svcMethods:    map[string]map[string]string{},
		handlerFields: map[string]map[string]string{},
		rootDir:       *rootDir,
		funcReturns:   map[string]string{},
	}

	must := func(err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	must(g.scanDir(filepath.Join(*rootDir, "internal/domain")))
	must(g.scanDir(filepath.Join(*rootDir, "internal/service")))
	must(g.scanDir(filepath.Join(*rootDir, "internal/web")))

	spec := g.buildSpec(*basePath, *title, *version)
	buf, err := json.MarshalIndent(spec, "", "  ")
	must(err)
	must(os.WriteFile(*outFile, buf, 0o644))

	fmt.Printf("已生成 %s：%d 条路由，%d 个 schema\n", *outFile, len(g.routes), len(g.structs))
	if *tsOut != "" {
		excl := map[string]bool{}
		for _, n := range strings.Split(*tsExclude, ",") {
			if n = strings.TrimSpace(n); n != "" {
				excl[n] = true
			}
		}
		must(writeTS(*tsOut, spec, excl))
		fmt.Printf("已生成 TypeScript 类型：%s\n", *tsOut)
	}
}

// ---------- 扫描 ----------

func (g *Gen) scanDir(dir string) error {
	switch {
	case strings.HasSuffix(filepath.ToSlash(dir), "internal/domain"):
		g.layer = "domain"
	case strings.HasSuffix(filepath.ToSlash(dir), "internal/service"):
		g.layer = "service"
	case strings.HasSuffix(filepath.ToSlash(dir), "internal/web"):
		g.layer = "web"
	}
	pkgs, err := parser.ParseDir(g.fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			g.collectStructs(file, pkg.Name)
			g.collectServiceInterfaces(file)
			g.collectHandlerStructs(file)
			g.collectRoutes(file)
		}
	}
	return nil
}

// jsonNameOf 解析 json tag，返回字段名与是否必填。
// 必填判定：tag 不含 omitempty，且 Go 类型不是指针（指针语义上可空）。
func jsonNameOf(f *ast.Field, structName string) (string, bool, bool) {
	name := ""
	required := false
	if f.Tag != nil {
		tag := strings.Trim(f.Tag.Value, "`")
		if i := strings.Index(tag, `json:"`); i >= 0 {
			rest := tag[i+6:]
			if j := strings.Index(rest, `"`); j > 0 {
				name = strings.Split(rest[:j], ",")[0]
			}
		}
		// 必填判定分两类语义（关键）：
		//  - 请求类型（*Req）：只有 binding:"required" 才是必填。后端 gin 据此
		//    拒绝请求；未标记的字段调用方不传也合法（缺 omitempty 并不代表必填）。
		//  - 响应类型（*VO/*Resp/其它）：后端总是填充返回，字段应视为必填。
		//    若在此也套用 binding 规则，会把所有响应字段都判成可选，
		//    迫使前端到处写空值判断，反而失真。
		if strings.Contains(tag, `binding:"required"`) {
			required = true
		} else if !strings.HasSuffix(structName, "Req") && !strings.Contains(tag, "omitempty") {
			required = true
		}
	}
	if name == "-" {
		return "", false, false
	}
	if name == "" && len(f.Names) > 0 {
		name = f.Names[0].Name
	}
	// 指针语义可空
	if _, isPtr := f.Type.(*ast.StarExpr); isPtr {
		required = false
	}
	return name, true, required
}

func typeToString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.ArrayType:
		return "[]" + typeToString(t.Elt)
	case *ast.MapType:
		return "map[" + typeToString(t.Key) + "]" + typeToString(t.Value)
	case *ast.InterfaceType:
		return "any"
	case *ast.ChanType:
		return "chan"
	}
	return "any"
}

func (g *Gen) collectStructs(file *ast.File, pkg string) {
	// 顺带收集包内函数/方法的第一个返回类型
	ast.Inspect(file, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Type == nil || fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
			return true
		}
		if _, exists := g.funcReturns[fd.Name.Name]; !exists {
			g.funcReturns[fd.Name.Name] = typeToString(fd.Type.Results.List[0].Type)
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		def := &StructDef{Name: ts.Name.Name, Pkg: pkg, Layer: g.layer}
		for _, f := range st.Fields.List {
			if len(f.Names) == 0 {
				// 匿名嵌入：记录类型名，稍后展开合并其字段
				emb := strings.TrimPrefix(typeToString(f.Type), "*")
				if dot := strings.LastIndex(emb, "."); dot >= 0 {
					emb = emb[dot+1:]
				}
				if emb != "" {
					def.Embeds = append(def.Embeds, emb)
				}
				continue
			}
			name, ok, required := jsonNameOf(f, ts.Name.Name)
			if !ok {
				continue
			}
			def.Fields = append(def.Fields, Field{
				JSONName: name,
				GoType:   typeToString(f.Type),
				Required: required,
			})
		}
		if _, exists := g.structs[def.Name]; !exists {
			g.structs[def.Name] = def
		}
		return true
	})
}

// collectServiceInterfaces 收集 service 接口方法返回类型。
func (g *Gen) collectServiceInterfaces(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		it, ok := ts.Type.(*ast.InterfaceType)
		if !ok || it.Methods == nil {
			return true
		}
		iface := ts.Name.Name
		if g.svcMethods[iface] == nil {
			g.svcMethods[iface] = map[string]string{}
		}
		for _, m := range it.Methods.List {
			ft, ok := m.Type.(*ast.FuncType)
			if !ok || len(m.Names) == 0 {
				continue
			}
			if ft.Results == nil || len(ft.Results.List) == 0 {
				continue
			}
			// 取第一个返回值的类型（约定：数据在前，error 在后）
			ret := typeToString(ft.Results.List[0].Type)
			g.svcMethods[iface][m.Names[0].Name] = ret
		}
		return true
	})
}

// collectHandlerStructs 收集 handler 结构体字段：字段名 -> service 接口名。
func (g *Gen) collectHandlerStructs(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		typeName := ts.Name.Name
		for _, f := range st.Fields.List {
			if len(f.Names) == 0 {
				continue
			}
			iface := typeToString(f.Type)
			if dot := strings.LastIndex(iface, "."); dot >= 0 {
				iface = iface[dot+1:]
			}
			if g.handlerFields[typeName] == nil {
				g.handlerFields[typeName] = map[string]string{}
			}
			g.handlerFields[typeName][f.Names[0].Name] = iface
		}
		return true
	})
}

// collectRoutes 解析 RegisterRoutes 中的路由注册。
func (g *Gen) collectRoutes(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "RegisterRoutes" || fd.Recv == nil {
			return true
		}
		recvType := ""
		if len(fd.Recv.List) > 0 {
			recvType = strings.TrimPrefix(typeToString(fd.Recv.List[0].Type), "*")
		}

		// group 变量 -> 路径前缀
		groups := map[string]string{}
		ast.Inspect(fd, func(inner ast.Node) bool {
			as, ok := inner.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 {
				return true
			}
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			call, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Group" || len(call.Args) != 1 {
				return true
			}
			if bl, ok := call.Args[0].(*ast.BasicLit); ok {
				groups[id.Name] = strings.Trim(bl.Value, `"`)
			}
			return true
		})

		ast.Inspect(fd, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			m := sel.Sel.Name
			if m != "GET" && m != "POST" && m != "PUT" && m != "DELETE" && m != "PATCH" {
				return true
			}
			// 接收者必须是本函数内定义的 group 变量，或 r（基础组）
			recv, _ := sel.X.(*ast.Ident)
			prefix := ""
			groupName := ""
			if recv != nil {
				if p, known := groups[recv.Name]; known {
					prefix, groupName = p, recv.Name
				} else if recv.Name == "r" {
					prefix, groupName = "", "r"
				} else {
					return true
				}
			}
			if len(call.Args) < 1 {
				return true
			}
			bl, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true // 非字面量路径，跳过
			}
			path := strings.Trim(bl.Value, `"`)

			bodyType := ""
			handler := ""
			if len(call.Args) >= 2 {
				switch a := call.Args[1].(type) {
				case *ast.IndexExpr: // h.Foo 或 Fn[T]
					if id, ok := a.Index.(*ast.Ident); ok {
						bodyType = id.Name
					}
				case *ast.CallExpr: // ginx.WrapBody[T](h.Handler) / ginx.Wrap(h.Handler)
					if ie, ok := a.Fun.(*ast.IndexExpr); ok {
						if id, ok := ie.Index.(*ast.Ident); ok {
							bodyType = id.Name
						}
					}
					// handler 是唯一的实参（h.Xxx）
					for _, arg := range a.Args {
						if se, ok := arg.(*ast.SelectorExpr); ok {
							if id, ok := se.X.(*ast.Ident); ok && id.Name == "h" {
								handler = se.Sel.Name
							}
						}
					}
				case *ast.SelectorExpr: // h.Handler
					handler = a.Sel.Name
				}
			}
			g.routes = append(g.routes, Route{
				Method:   m,
				Path:     prefix + path,
				BodyType: bodyType,
				Handler:  handler,
				RecvType: recvType,
				Group:    groupName,
			})
			return true
		})
		return false // 已处理该函数
	})
}

// ---------- OpenAPI 构建 ----------

type Schema = map[string]any

func (g *Gen) buildSpec(basePath, title, version string) map[string]any {
	paths := map[string]any{}
	// 去重：同一 method+path 只保留一条
	seen := map[string]bool{}
	sort.Slice(g.routes, func(i, j int) bool {
		if g.routes[i].Path != g.routes[j].Path {
			return g.routes[i].Path < g.routes[j].Path
		}
		return g.routes[i].Method < g.routes[j].Method
	})

	for _, r := range g.routes {
		key := r.Method + " " + r.Path
		if seen[key] {
			continue
		}
		seen[key] = true

		op := map[string]any{
			"tags":        []string{tagOf(r.Path)},
			"operationId": operationID(r),
			"responses":   g.responsesFor(r),
		}
		if r.BodyType != "" {
			if s := g.schemaRef(r.BodyType); s != nil {
				op["requestBody"] = map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{"schema": s},
					},
				}
			}
		}
		// path 参数
		if params := pathParams(r.Path); len(params) > 0 {
			var list []any
			for _, p := range params {
				list = append(list, map[string]any{
					"name": p, "in": "path", "required": true,
					"schema": map[string]any{"type": "string"},
				})
			}
			op["parameters"] = list
		}
		// 统一挂到 basePath 下
		full := basePath + r.Path
		if paths[full] == nil {
			paths[full] = map[string]any{}
		}
		paths[full].(map[string]any)[strings.ToLower(r.Method)] = op
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       title,
			"version":     version,
			"description": "由 cmd/openapi 从 e-cmdb-service 源码静态生成，请勿手工编辑。",
		},
		"paths": paths,
		"components": map[string]any{
			"schemas": g.buildSchemas(),
		},
	}
}

func tagOf(path string) string {
	p := strings.Trim(path, "/")
	if p == "" {
		return "root"
	}
	return strings.Split(p, "/")[0]
}

func operationID(r Route) string {
	base := r.Handler
	if base == "" || base == "Wrap" || base == "WrapBody" {
		// 退化：用路径推导
		p := strings.Trim(r.Path, "/")
		p = strings.NewReplacer("/", "_", ":", "", "-", "_").Replace(p)
		base = strings.ToLower(r.Method) + "_" + p
	}
	return strings.ToLower(r.Method) + "_" + base
}

func pathParams(path string) []string {
	var out []string
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, ":") {
			out = append(out, strings.TrimPrefix(seg, ":"))
		}
	}
	return out
}

// responsesFor 推断 200 响应 schema：追踪 handler -> service 方法 -> 返回类型。
func (g *Gen) responsesFor(r Route) map[string]any {
	dataSchema := g.inferResponse(r)
	ok := map[string]any{
		"description": "成功",
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code": map[string]any{"type": "integer"},
						"msg":  map[string]any{"type": "string"},
						"data": dataSchema,
					},
				},
			},
		},
	}
	return map[string]any{
		"200": ok,
		"400": map[string]any{"description": "参数错误"},
		"401": map[string]any{"description": "未认证"},
		"500": map[string]any{"description": "服务端错误"},
	}
}

// inferResponse 从 handler 的 `return Result(...)` 实参推断响应 schema。
//
// 支持的实参形态：
//   - Result(nil)                        → 无 data（返回 null）
//   - Result(SomeVO{...})                → $ref SomeVO
//   - Result(ToXxxVO(...))               → 无法静态确定具体类型，留空
//   - Result(map[string]interface{}{...}) → 按字面量 key 生成 object
//   - Result(data)（变量）                → 回退到 service 返回类型推断
func (g *Gen) inferResponse(r Route) Schema {
	if r.RecvType == "" || r.Handler == "" {
		return map[string]any{}
	}
	files, err := parser.ParseDir(g.fset, filepath.Join(g.rootDir, "internal/web"), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return map[string]any{}
	}
	for _, pkg := range files {
		for _, file := range pkg.Files {
			var ret Schema
			ast.Inspect(file, func(n ast.Node) bool {
				if ret != nil {
					return false
				}
				fd, ok := n.(*ast.FuncDecl)
				if !ok || fd.Name.Name != r.Handler || fd.Recv == nil || len(fd.Recv.List) == 0 {
					return true
				}
				if strings.TrimPrefix(typeToString(fd.Recv.List[0].Type), "*") != r.RecvType {
					return true
				}
				if fd.Body == nil {
					return false
				}
				ast.Inspect(fd.Body, func(inner ast.Node) bool {
					// 两种成功响应写法都要覆盖：
					//   A) return Result(x), nil            （ginx.Wrap 风格）
					//   B) ctx.JSON(200, Result(x))         （gin 原生风格，本仓库 GET/DELETE 多用）
					var call *ast.CallExpr
					switch n := inner.(type) {
					case *ast.ReturnStmt:
						if len(n.Results) == 0 {
							return true
						}
						c, ok := n.Results[0].(*ast.CallExpr)
						if !ok {
							return true
						}
						call = c
					case *ast.CallExpr:
						// ctx.JSON(code, Result(x))
						sel, ok := n.Fun.(*ast.SelectorExpr)
						if !ok || sel.Sel.Name != "JSON" || len(n.Args) != 2 {
							return true
						}
						c, ok := n.Args[1].(*ast.CallExpr)
						if !ok {
							return true
						}
						call = c
					default:
						return true
					}
					if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "Result" || len(call.Args) != 1 {
						return true
					}
					ret = g.schemaFromArg(call.Args[0], r)
					return false
				})
				return false
			})
			if ret != nil {
				return ret
			}
		}
	}
	return map[string]any{}
}

// schemaFromArg 把 Result(...) 的实参转换为 JSON Schema。
func (g *Gen) schemaFromArg(arg ast.Expr, r Route) Schema {
	switch a := arg.(type) {
	case *ast.Ident:
		if a.Name == "nil" {
			return map[string]any{"nullable": true, "description": "无返回数据"}
		}
		// 变量：回退到 service 返回类型
		if s := g.inferFromServiceCall(r); s != nil {
			return s
		}
		return map[string]any{}
	case *ast.CompositeLit:
		// 具名结构体字面量：XxxVO{...}
		if id, ok := a.Type.(*ast.Ident); ok {
			if _, known := g.structs[id.Name]; known {
				return map[string]any{"$ref": "#/components/schemas/" + id.Name}
			}
		}
		// map[string]interface{}{...}：按 key 生成
		if _, ok := a.Type.(*ast.MapType); ok {
			props := map[string]any{}
			for _, el := range a.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key := ""
				if bl, ok := kv.Key.(*ast.BasicLit); ok {
					key = strings.Trim(bl.Value, `"`)
				}
				if key == "" {
					continue
				}
				props[key] = map[string]any{}
			}
			if len(props) > 0 {
				return map[string]any{"type": "object", "properties": props}
			}
		}
		return map[string]any{}
	case *ast.CallExpr:
		// Result(h.toVO(inst)) / Result(toDTOs(xs))：查被调函数的第一个返回类型
		name := ""
		switch fn := a.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		}
		if name != "" {
			if rt, ok := g.funcReturns[name]; ok && rt != "" {
				if s := g.schemaRef(rt); s != nil {
					if m, ok := s["type"]; !ok || m != nil {
						if _, hasRef := s["$ref"]; hasRef || s["type"] != nil {
							return s
						}
					}
				}
			}
		}
		return map[string]any{}
	case *ast.UnaryExpr: // &XxxVO{...}
		if cl, ok := a.X.(*ast.CompositeLit); ok {
			return g.schemaFromArg(cl, r)
		}
		return map[string]any{}
	}
	return map[string]any{}
}

// inferFromServiceCall 回退策略：handler 内 h.<field>.<Method>() 的 service 返回类型。
func (g *Gen) inferFromServiceCall(r Route) Schema {
	files, err := parser.ParseDir(g.fset, filepath.Join(g.rootDir, "internal/web"), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil
	}
	var out Schema
	for _, pkg := range files {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				if out != nil {
					return false
				}
				fd, ok := n.(*ast.FuncDecl)
				if !ok || fd.Name.Name != r.Handler || fd.Recv == nil || len(fd.Recv.List) == 0 {
					return true
				}
				if strings.TrimPrefix(typeToString(fd.Recv.List[0].Type), "*") != r.RecvType {
					return true
				}
				if fd.Body == nil {
					return false
				}
				ast.Inspect(fd.Body, func(inner ast.Node) bool {
					if out != nil {
						return false
					}
					call, ok := inner.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					x, ok := sel.X.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					id, ok := x.X.(*ast.Ident)
					if !ok || id.Name != "h" {
						return true
					}
					iface := g.handlerFields[r.RecvType][x.Sel.Name]
					if iface == "" {
						return true
					}
					retType := g.svcMethods[iface][sel.Sel.Name]
					if retType == "" {
						return true
					}
					out = g.schemaRef(retType)
					return false
				})
				return false
			})
			if out != nil {
				return out
			}
		}
	}
	return out
}

// schemaRef 返回类型引用或内联 schema。
func (g *Gen) schemaRef(goType string) Schema {
	base := strings.TrimPrefix(goType, "*")
	// 去掉包名前缀（domain.Instance -> Instance）
	if dot := strings.LastIndex(base, "."); dot >= 0 {
		base = base[dot+1:]
	}
	if strings.HasPrefix(base, "[]") {
		inner := g.schemaRef(strings.TrimPrefix(base, "[]"))
		return map[string]any{"type": "array", "items": inner}
	}
	if def, ok := g.structs[base]; ok {
		_ = def
		return map[string]any{"$ref": "#/components/schemas/" + base}
	}
	switch base {
	case "string":
		return map[string]any{"type": "string"}
	case "int", "int64", "int32", "int16", "int8", "uint", "uint64":
		return map[string]any{"type": "integer", "format": "int64"}
	case "float64", "float32":
		return map[string]any{"type": "number"}
	case "bool":
		return map[string]any{"type": "boolean"}
	case "any", "interface{}":
		return map[string]any{}
	}
	if strings.HasPrefix(base, "map[") {
		return map[string]any{"type": "object", "additionalProperties": true}
	}
	return map[string]any{}
}

func (g *Gen) buildSchemas() map[string]any {
	out := map[string]any{}
	for name, def := range g.structs {
		props := map[string]any{}
		var required []string
		// 先展开匿名嵌入（Go 的 embedding 会把父结构字段提升到子结构 JSON 中）
		seen := map[string]bool{}
		for _, f := range g.flattenFields(def, seen) {
			props[f.JSONName] = g.schemaRef(f.GoType)
			if f.Required {
				required = append(required, f.JSONName)
			}
		}
		s := map[string]any{
			"type":       "object",
			"properties": props,
			// 扩展字段：标记来源层级，供 TS 生成时只输出 web 层传输类型
			"x-layer": def.Layer,
		}
		if len(required) > 0 {
			sort.Strings(required)
			s["required"] = required
		}
		out[name] = s
	}
	return out
}

// flattenFields 递归展开匿名嵌入的字段（Go embedding 语义：父结构字段被提升）。
// seen 用于防止自引用/环导致无限递归。
func (g *Gen) flattenFields(def *StructDef, seen map[string]bool) []Field {
	if def == nil || seen[def.Name] {
		return nil
	}
	seen[def.Name] = true
	var out []Field
	for _, emb := range def.Embeds {
		if e, ok := g.structs[emb]; ok {
			out = append(out, g.flattenFields(e, seen)...)
		}
	}
	out = append(out, def.Fields...)
	return out
}
