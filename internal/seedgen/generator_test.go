package seedgen

import (
	"testing"

	shareddomain "github.com/Havens-blog/e-cloudx-sdk/domain"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

func TestGenerateInstances_ProducesValidInstances(t *testing.T) {
	models := []Model{
		{UID: "aliyun_ecs", Name: "云服务器 ECS", Provider: "aliyun"},
		{UID: "cloud_vm", Name: "云主机", Provider: "all"},
		{UID: "tencent_rds", Name: "云数据库 MySQL", Provider: "tencent"},
	}

	for _, m := range models {
		instances := GenerateInstances(m, 1, 100, 3, "seed")
		if len(instances) != 3 {
			t.Fatalf("%s: expected 3 instances, got %d", m.UID, len(instances))
		}
		for i, inst := range instances {
			if err := inst.Validate(); err != nil {
				t.Fatalf("%s[%d]: instance failed Validate: %v", m.UID, i, err)
			}
			if inst.ModelUID != m.UID {
				t.Fatalf("%s[%d]: ModelUID mismatch: %s", m.UID, i, inst.ModelUID)
			}
			if inst.TenantID != 1 || inst.AccountID != 100 {
				t.Fatalf("%s[%d]: tenant/account not propagated", m.UID, i)
			}
			if inst.AssetID == "" || inst.AssetName == "" {
				t.Fatalf("%s[%d]: asset id/name must not be empty", m.UID, i)
			}
			if _, ok := inst.Attributes["provider"]; !ok {
				t.Fatalf("%s[%d]: missing provider attribute", m.UID, i)
			}
			if _, ok := inst.Attributes["region"]; !ok {
				t.Fatalf("%s[%d]: missing region attribute", m.UID, i)
			}
			if _, ok := inst.Attributes["status"]; !ok {
				t.Fatalf("%s[%d]: missing status attribute", m.UID, i)
			}
		}
	}
}

func TestGenerateInstances_IsDeterministic(t *testing.T) {
	m := Model{UID: "aliyun_ecs", Name: "云服务器 ECS", Provider: "aliyun"}
	a := GenerateInstances(m, 1, 1, 3, "seed")
	b := GenerateInstances(m, 1, 1, 3, "seed")

	if len(a) != len(b) {
		t.Fatalf("length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].AssetID != b[i].AssetID {
			t.Fatalf("asset id not deterministic at %d: %q vs %q", i, a[i].AssetID, b[i].AssetID)
		}
	}
}

func TestGenerateInstances_DefaultCount(t *testing.T) {
	m := Model{UID: "aws_redis", Name: "Redis", Provider: "aws"}
	got := GenerateInstances(m, 1, 1, 0, "")
	if len(got) != DefaultPerModel {
		t.Fatalf("expected default count %d, got %d", DefaultPerModel, len(got))
	}
	if got[0].AssetID != "seed-aws_redis-001" {
		t.Fatalf("expected default prefix, got %q", got[0].AssetID)
	}
}

func TestSampleAttributes_AssetTypeExtraction(t *testing.T) {
	// 通用 model_uid 也应被正确归一为资源类型。
	if got := shareddomain.ExtractAssetType("cloud_vm"); got != "ecs" {
		t.Fatalf("expected ecs, got %s", got)
	}
	if got := shareddomain.ExtractAssetType("aliyun_slb"); got != "slb" {
		t.Fatalf("expected slb, got %s", got)
	}

	attrs := SampleAttributes("ecs", "aliyun", 1)
	if attrs["instance_type"] == nil {
		t.Fatalf("ecs should have instance_type, got %#v", attrs)
	}
	if attrs["provider"] != "aliyun" {
		t.Fatalf("provider mismatch: %v", attrs["provider"])
	}
}

func TestProviderValue_Normalization(t *testing.T) {
	cases := map[string]string{
		"":        "generic",
		"all":     "generic",
		"aliyun":  "aliyun",
		" aws ":   "aws",
		"tencent": "tencent",
	}
	for in, want := range cases {
		if got := providerValue(in); got != want {
			t.Fatalf("providerValue(%q) = %q, want %q", in, got, want)
		}
	}
}

// 确保生成的实例属于 domain.Instance 类型（编译期断言）。
var _ []domain.Instance = GenerateInstances(Model{UID: "aliyun_ecs"}, 1, 1, 1, "")
