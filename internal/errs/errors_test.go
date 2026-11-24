package errs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"
)

// TestErrorCodeUnique 锁定「同一 code 不得承载两种业务语义」。
// 起因：删除保护（RelationTypeInUse）与模型对校验（RelationModelMismatch）
// 两个并行 PR 各自选了 400010，合并后对外撞号，前端按 code 分支无法区分。
// 新增错误码时请同步登记到 all —— 下面的 declaredErrorCodeNames 会直接读本包
// errors.go 的源码做覆盖性校验，漏登记会让本测试失败。
func TestErrorCodeUnique(t *testing.T) {
	all := []struct {
		name string
		ec   ErrorCode
	}{
		{"Success", Success},
		{"ParamsError", ParamsError},
		{"SystemError", SystemError},
		{"ModelNotFound", ModelNotFound},
		{"ModelExists", ModelExists},
		{"ModelInvalid", ModelInvalid},
		{"ModelHasInstances", ModelHasInstances},
		{"RelationTypeInUse", RelationTypeInUse},
		{"InstanceNotFound", InstanceNotFound},
		{"InstanceExists", InstanceExists},
		{"InstanceInvalid", InstanceInvalid},
		{"RelationNotFound", RelationNotFound},
		{"RelationExists", RelationExists},
		{"RelationInvalid", RelationInvalid},
		{"RelationModelMismatch", RelationModelMismatch},
		{"AttributeNotFound", AttributeNotFound},
		{"AttributeExists", AttributeExists},
		{"AttributeInvalid", AttributeInvalid},
		{"AttributeInUse", AttributeInUse},
		{"ModelGroupNotFound", ModelGroupNotFound},
		{"ModelGroupExists", ModelGroupExists},
		{"ModelGroupInvalid", ModelGroupInvalid},
		{"GroupHasModels", GroupHasModels},
		{"CannotDeleteBuiltin", CannotDeleteBuiltin},
		{"OpenAPITokenNotFound", OpenAPITokenNotFound},
		{"OpenAPIWebhookNotFound", OpenAPIWebhookNotFound},
	}

	registered := make(map[string]ErrorCode, len(all))
	for _, item := range all {
		if _, dup := registered[item.name]; dup {
			t.Fatalf("登记表里有重复的名称：%s", item.name)
		}
		registered[item.name] = item.ec
	}

	// 覆盖性校验：直接扫 errors.go 的声明，确保「声明了但没登记」也会被发现。
	// 早前的实现只在表内去重，注释里写的「反射式兜底」并不存在，
	// 于是新增的错误码漏登记也不会报错（评审指出）。
	declared := declaredErrorCodeNames(t)
	for _, name := range declared {
		if _, ok := registered[name]; !ok {
			t.Fatalf("errors.go 声明了 %s 但未登记到 all，请补上", name)
		}
	}
	for name := range registered {
		if !containsName(declared, name) {
			t.Fatalf("all 里登记了 %s，但 errors.go 中已不存在该声明，请删除", name)
		}
	}

	seen := make(map[int]string, len(all))
	for _, item := range all {
		if prev, dup := seen[item.ec.Code]; dup {
			t.Fatalf("错误码重复：%s 与 %s 都使用 %d", prev, item.name, item.ec.Code)
		}
		seen[item.ec.Code] = item.name
	}
}

// declaredErrorCodeNames 解析本包 errors.go，收集所有 `var X = ErrorCode{...}` 的变量名。
// 用 AST 而不是硬编码列表，才能真正兜住「新增错误码却忘记登记」。
func declaredErrorCodeNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "errors.go", nil, 0)
	if err != nil {
		t.Fatalf("解析 errors.go 失败：%v", err)
	}
	names := make([]string, 0, 32)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Values) == 0 {
				continue
			}
			lit, ok := value.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			if ident, ok := lit.Type.(*ast.Ident); !ok || ident.Name != "ErrorCode" {
				continue
			}
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("errors.go 里没有解析到任何 ErrorCode 声明，测试失去意义")
	}
	sort.Strings(names)
	return names
}

func containsName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}
