package domain

import "testing"

// TestModelRelationType_ValidateNormalizes 锁定「裁剪下沉到 domain」的契约。
//
// 背景：裁剪此前只写在 service.Create/Update 里，而 Validate 是导出方法，
// 直接调用它的路径（迁移脚本、将来的批处理）会把 " " 当合法名称落库，
// 结果列表与拓扑边标签全部空白。现在 Validate 内部先 Normalize，任何入口都安全。
func TestModelRelationType_ValidateNormalizes(t *testing.T) {
	rel := ModelRelationType{
		UID:            "  ecs_belong_to_vpc  ",
		Name:           "  云服务器属于 VPC  ",
		SourceModelUID: " ecs ",
		TargetModelUID: "\tvpc\n",
		SourceToTarget: "  属于  ",
		TargetToSource: "  包含  ",
		Description:    "  迁移时补齐的存量类型  ",
	}
	if err := rel.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	want := ModelRelationType{
		UID:            "ecs_belong_to_vpc",
		Name:           "云服务器属于 VPC",
		SourceModelUID: "ecs",
		TargetModelUID: "vpc",
		SourceToTarget: "属于",
		TargetToSource: "包含",
		Description:    "迁移时补齐的存量类型",
	}
	if rel.UID != want.UID || rel.Name != want.Name ||
		rel.SourceModelUID != want.SourceModelUID || rel.TargetModelUID != want.TargetModelUID ||
		rel.SourceToTarget != want.SourceToTarget || rel.TargetToSource != want.TargetToSource ||
		rel.Description != want.Description {
		t.Fatalf("Validate() 未按预期裁剪：got %+v, want %+v", rel, want)
	}
}

// TestModelRelationType_ValidateRejectsBlankAfterTrim 锁定「纯空白必被判非法」。
func TestModelRelationType_ValidateRejectsBlankAfterTrim(t *testing.T) {
	base := ModelRelationType{
		UID:            "ecs_vpc",
		Name:           "名称",
		SourceModelUID: "ecs",
		TargetModelUID: "vpc",
	}
	cases := []struct {
		name   string
		mutate func(*ModelRelationType)
		want   error
	}{
		{"uid 纯空白", func(m *ModelRelationType) { m.UID = "   " }, ErrInvalidRelationTypeUID},
		{"name 纯空白", func(m *ModelRelationType) { m.Name = " \t " }, ErrInvalidRelationName},
		{"源模型纯空白", func(m *ModelRelationType) { m.SourceModelUID = "  " }, ErrInvalidRelationModel},
		{"目标模型纯空白", func(m *ModelRelationType) { m.TargetModelUID = " \n" }, ErrInvalidRelationModel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel := base
			tc.mutate(&rel)
			if err := rel.Validate(); err != tc.want {
				t.Fatalf("Validate() error = %v, want %v", err, tc.want)
			}
		})
	}
}
