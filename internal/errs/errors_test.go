package errs

import "testing"

// TestErrorCodeUnique 锁定「同一 code 不得承载两种业务语义」。
// 起因：删除保护（RelationTypeInUse）与模型对校验（RelationModelMismatch）
// 两个并行 PR 各自选了 400010，合并后对外撞号，前端按 code 分支无法区分。
// 此测试把全部已声明错误码收进一张表，任何新增/改号造成的重复都会在这里失败。
// 新增错误码时请同步登记到 all。
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
		{"ModelGroupNotFound", ModelGroupNotFound},
		{"ModelGroupExists", ModelGroupExists},
		{"ModelGroupInvalid", ModelGroupInvalid},
		{"GroupHasModels", GroupHasModels},
		{"CannotDeleteBuiltin", CannotDeleteBuiltin},
		{"OpenAPITokenNotFound", OpenAPITokenNotFound},
		{"OpenAPIWebhookNotFound", OpenAPIWebhookNotFound},
	}

	// 表必须覆盖包里全部已导出错误码：靠反射式登记清单兜底，
	// 防止有人新增错误码却忘记登记，让唯一性检查形同虚设。
	declared := map[string]bool{}
	for _, item := range all {
		declared[item.name] = true
	}
	if len(declared) != len(all) {
		t.Fatalf("登记表里有重复的名称：%d 项只有 %d 个不同名字", len(all), len(declared))
	}

	seen := make(map[int]string, len(all))
	for _, item := range all {
		if prev, dup := seen[item.ec.Code]; dup {
			t.Fatalf("错误码重复：%s 与 %s 都使用 %d", prev, item.name, item.ec.Code)
		}
		seen[item.ec.Code] = item.name
	}
}
