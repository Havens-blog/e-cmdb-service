// Package seedgen 提供为 CMDB 模型生成示例实例（Instance）的纯函数逻辑。
//
// 该包不依赖任何数据库连接，便于单元测试；实际落库由 cmd/seed 命令负责。
// 造数逻辑说明：
//   - 实例与模型的关联字段是 Instance.ModelUID（对应 Model.UID）。
//   - AssetID 使用确定性的 "seed-<modelUID>-<序号>"，配合「先查后插」实现幂等：
//     重复执行时已存在的实例会被跳过，不会重复插入。
//   - Attributes 为动态属性 map，按资源类型（由 modelUID 提取）生成合理示例值，
//     统一带上 provider / region / status 三个通用键，与列表过滤、按厂商统计的口径一致。
package seedgen

import (
	"fmt"
	"strings"

	shareddomain "github.com/Havens-blog/e-cloudx-sdk/domain"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// DefaultAssetPrefix 造数实例 AssetID 的默认前缀。
const DefaultAssetPrefix = "seed"

// DefaultPerModel 每个模型默认生成的实例数量。
const DefaultPerModel = 3

// Model 生成实例所需的最小模型信息。
type Model struct {
	UID      string // 模型唯一标识，如 aliyun_ecs / cloud_vm
	Name     string // 模型名称，用于生成 AssetName
	Provider string // 云厂商（all 表示通用）
}

// GenerateInstances 为单个模型生成 count 条示例实例。
// 结果是确定性的：相同入参（同一 UID、prefix、序号）生成完全相同的 AssetID，
// 从而保证「先查后插」的幂等语义。
func GenerateInstances(m Model, tenantID, accountID int64, count int, assetPrefix string) []domain.Instance {
	if count <= 0 {
		count = DefaultPerModel
	}
	if assetPrefix == "" {
		assetPrefix = DefaultAssetPrefix
	}

	provider := providerValue(m.Provider)
	assetType := shareddomain.ExtractAssetType(m.UID)
	displayName := m.Name
	if displayName == "" {
		displayName = m.UID
	}

	instances := make([]domain.Instance, 0, count)
	for i := 1; i <= count; i++ {
		instances = append(instances, domain.Instance{
			ModelUID:   m.UID,
			AssetID:    fmt.Sprintf("%s-%s-%03d", assetPrefix, m.UID, i),
			AssetName:  fmt.Sprintf("%s-示例-%02d", displayName, i),
			TenantID:   tenantID,
			AccountID:  accountID,
			Attributes: SampleAttributes(assetType, provider, i),
		})
	}
	return instances
}

// providerValue 归一化云厂商标识：空或 "all"（通用模型）归一为 "generic"。
func providerValue(modelProvider string) string {
	p := strings.TrimSpace(modelProvider)
	if p == "" || p == "all" {
		return "generic"
	}
	return p
}

// SampleAttributes 按资源类型生成示例属性值。
// provider/region/status 为所有类型的通用键（与实例列表过滤、按厂商统计口径一致），
// 其余键按资源类型补充合理的示例值。
func SampleAttributes(assetType, provider string, i int) map[string]interface{} {
	attrs := map[string]interface{}{
		"provider":    provider,
		"region":      "cn-hangzhou",
		"status":      "running",
		"description": fmt.Sprintf("seed 示例实例 #%d", i),
	}

	// i 归一化到 1..250 之间，用于生成看起来合理的 IP 等示例值。
	seg := (i-1)%250 + 1

	switch assetType {
	case "ecs":
		attrs["instance_type"] = "ecs.g7.large"
		attrs["instance_type_family"] = "ecs.g7"
		attrs["cpu"] = 4
		attrs["memory"] = 8192
		attrs["os_type"] = "linux"
		attrs["os_name"] = "Alibaba Cloud Linux 3.2104 LTS"
		attrs["zone"] = "cn-hangzhou-i"
		attrs["charge_type"] = "PostPaid"
		attrs["private_ip"] = fmt.Sprintf("10.0.%d.%d", seg%250, seg%250+1)
		attrs["public_ip"] = fmt.Sprintf("120.55.%d.%d", seg%250, seg%250+1)
		attrs["image_id"] = "aliyun_3_x64_20G_alibase"
		attrs["host_name"] = fmt.Sprintf("seed-host-%02d", i)
	case "disk":
		attrs["status"] = "In_use"
		attrs["category"] = "cloud_essd"
		attrs["size"] = 100 + (i%5)*20
		attrs["zone"] = "cn-hangzhou-i"
		attrs["device"] = "/dev/xvdb"
	case "snapshot":
		attrs["status"] = "accomplished"
		attrs["progress"] = "100%"
		attrs["source_disk_id"] = fmt.Sprintf("d-bp1seed%02d", i)
		attrs["category"] = "cloud_essd"
	case "security_group":
		attrs["status"] = "available"
		attrs["vpc_id"] = "vpc-bp1seed0001"
		attrs["region"] = "cn-hangzhou"
	case "image":
		attrs["status"] = "Available"
		attrs["os_name"] = "Alibaba Cloud Linux 3.2104 LTS"
		attrs["platform"] = "Linux"
		attrs["size"] = 20
	case "rds":
		attrs["engine"] = "MySQL"
		attrs["engine_version"] = "8.0"
		attrs["instance_class"] = "mysql.n4.large.1"
		attrs["storage"] = 200
		attrs["connection_string"] = fmt.Sprintf("rm-seed%02d.mysql.rds.aliyuncs.com", i)
		attrs["port"] = 3306
		attrs["category"] = "HighAvailability"
	case "redis":
		attrs["engine_version"] = "5.0"
		attrs["instance_class"] = "redis.master.small.default"
		attrs["capacity"] = 1024
		attrs["connection_string"] = fmt.Sprintf("r-seed%02d.redis.rds.aliyuncs.com", i)
		attrs["port"] = 6379
	case "mongodb":
		attrs["engine_version"] = "4.4"
		attrs["instance_class"] = "dds.mongo.mid"
		attrs["storage"] = 100
		attrs["connection_string"] = fmt.Sprintf("dds-seed%02d.mongodb.rds.aliyuncs.com", i)
		attrs["port"] = 3717
	case "vpc":
		attrs["status"] = "Available"
		attrs["cidr_block"] = fmt.Sprintf("10.%d.0.0/16", seg)
		attrs["region"] = "cn-hangzhou"
	case "vswitch", "subnet":
		attrs["status"] = "Available"
		attrs["cidr_block"] = fmt.Sprintf("10.%d.%d.0/24", seg, seg)
		attrs["zone"] = "cn-hangzhou-i"
		attrs["vpc_id"] = "vpc-bp1seed0001"
	case "eip":
		attrs["status"] = "InUse"
		attrs["ip_address"] = fmt.Sprintf("120.55.%d.%d", seg, seg+1)
		attrs["bandwidth"] = 100
		attrs["internet_charge_type"] = "PayByTraffic"
		attrs["instance_id"] = "i-bp1seed0001"
	case "eni":
		attrs["status"] = "Available"
		attrs["mac_address"] = fmt.Sprintf("00:16:3e:00:00:%02x", seg)
		attrs["private_ip"] = fmt.Sprintf("10.0.%d.%d", seg%250, seg%250+1)
		attrs["vswitch_id"] = "vsw-bp1seed0001"
	case "lb", "slb", "alb", "nlb", "elb", "clb":
		attrs["status"] = "Active"
		attrs["address"] = fmt.Sprintf("120.55.%d.%d", seg, seg+2)
		attrs["network_type"] = "vpc"
		attrs["bandwidth"] = 100
		attrs["vpc_id"] = "vpc-bp1seed0001"
	case "cdn":
		attrs["status"] = "online"
		attrs["domain"] = fmt.Sprintf("seed%02d.example.com", i)
		attrs["cname"] = fmt.Sprintf("seed%02d.example.com.w.cdngslb.com", i)
		attrs["origin_type"] = "domain"
	case "waf":
		attrs["status"] = "running"
		attrs["domain"] = fmt.Sprintf("seed%02d.example.com", i)
		attrs["version"] = "pro"
	case "dns":
		attrs["domain"] = fmt.Sprintf("seed%02d.example.com", i)
		attrs["record_type"] = "A"
		attrs["value"] = fmt.Sprintf("120.55.%d.%d", seg, seg+1)
		attrs["ttl"] = 600
	case "nas":
		attrs["status"] = "Running"
		attrs["protocol"] = "NFS"
		attrs["storage_type"] = "Performance"
		attrs["capacity"] = 100
		attrs["mount_target"] = fmt.Sprintf("seed%02d.cn-hangzhou.nas.aliyuncs.com", i)
	case "oss":
		attrs["status"] = "running"
		attrs["bucket_name"] = fmt.Sprintf("seed-bucket-%02d", i)
		attrs["endpoint"] = "oss-cn-hangzhou.aliyuncs.com"
		attrs["storage_class"] = "Standard"
	case "kafka":
		attrs["version"] = "2.6.2"
		attrs["disk_type"] = "1"
		attrs["disk_size"] = 500
		attrs["endpoint"] = fmt.Sprintf("alikafka-seed%02d.cn-hangzhou.aliyuncs.com", i)
	case "elasticsearch":
		attrs["version"] = "7.10.0"
		attrs["node_spec"] = "elasticsearch.sn2ne.large"
		attrs["node_amount"] = 3
		attrs["endpoint"] = fmt.Sprintf("es-seed%02d.public.elasticsearch.aliyuncs.com", i)

	// 以下类型不在 e-cloudx-sdk 的资源注册表中，ExtractAssetType 会原样返回 modelUID，
	// 因此 case 使用完整 modelUID 精确匹配。
	case "host":
		attrs["status"] = "running"
		attrs["cpu"] = 32
		attrs["memory"] = 131072
		attrs["os_name"] = "CentOS 7.9"
		attrs["private_ip"] = fmt.Sprintf("10.0.%d.%d", seg%250, seg%250+1)
		attrs["zone"] = "cn-hangzhou-i"
		attrs["host_type"] = "physical"
	case "nat":
		attrs["status"] = "Available"
		attrs["vpc_id"] = "vpc-bp1seed0001"
		attrs["spec"] = "small"
	case "route_table":
		attrs["status"] = "Available"
		attrs["vpc_id"] = "vpc-bp1seed0001"
		attrs["route_count"] = 3 + i%5
	case "acl":
		attrs["status"] = "Available"
		attrs["vpc_id"] = "vpc-bp1seed0001"
		attrs["rule_count"] = 2 + i%4
	case "certificate":
		attrs["status"] = "issued"
		attrs["domain"] = fmt.Sprintf("seed%02d.example.com", i)
		attrs["cert_type"] = "DV"
		attrs["issuer"] = "Let's Encrypt"
		attrs["expire_time"] = "2027-12-31 23:59:59"
	case "s3", "aws_s3":
		attrs["status"] = "running"
		attrs["bucket_name"] = fmt.Sprintf("seed-bucket-%02d", i)
		attrs["endpoint"] = "s3.cn-north-1.amazonaws.com.cn"
		attrs["storage_class"] = "Standard"

	// IAM 家族（通用 cloud 与 aws 前缀两种 modelUID）。
	case "iam_user", "aws_iam_user":
		attrs["status"] = "Active"
		attrs["user_name"] = fmt.Sprintf("seed-user-%02d", i)
		attrs["display_name"] = fmt.Sprintf("示例用户 %02d", i)
	case "iam_group", "aws_iam_group":
		attrs["status"] = "Active"
		attrs["group_name"] = fmt.Sprintf("seed-group-%02d", i)
	case "iam_role", "aws_iam_role":
		attrs["status"] = "Active"
		attrs["role_name"] = fmt.Sprintf("seed-role-%02d", i)
	case "iam_policy", "aws_iam_policy":
		attrs["status"] = "Active"
		attrs["policy_name"] = fmt.Sprintf("seed-policy-%02d", i)
		attrs["policy_type"] = "Custom"

	// Kubernetes 家族。
	case "k8s_cluster":
		attrs["status"] = "running"
		attrs["version"] = "1.28.0"
		attrs["node_count"] = 3 + i%3
	case "k8s_node":
		attrs["status"] = "ready"
		attrs["instance_id"] = fmt.Sprintf("i-bp1seednode%02d", i)
		attrs["cpu"] = 8
		attrs["memory"] = 32768
		attrs["private_ip"] = fmt.Sprintf("10.1.%d.%d", seg%250, seg%250+1)
	case "k8s_pod":
		attrs["status"] = "Running"
		attrs["namespace"] = "default"
		attrs["pod_ip"] = fmt.Sprintf("10.2.%d.%d", seg%250, seg%250+1)
		attrs["node_name"] = "seed-node-01"
	case "k8s_namespace":
		attrs["status"] = "Active"
	case "k8s_deployment":
		attrs["status"] = "Available"
		attrs["namespace"] = "default"
		attrs["replicas"] = 2 + i%3
	case "k8s_service":
		attrs["status"] = "Active"
		attrs["namespace"] = "default"
		attrs["service_type"] = "ClusterIP"
		attrs["cluster_ip"] = fmt.Sprintf("10.96.0.%d", seg)

	// 中间件家族。
	case "rabbitmq":
		attrs["version"] = "3.8.14"
		attrs["instance_class"] = "amqp.small"
		attrs["endpoint"] = fmt.Sprintf("amqp-seed%02d.mq.cn-hangzhou.aliyuncs.com", i)
	case "rocketmq":
		attrs["version"] = "4.8.0"
		attrs["instance_class"] = "rmq.small"
		attrs["endpoint"] = fmt.Sprintf("rmq-seed%02d.mq.cn-hangzhou.aliyuncs.com", i)

	default:
		// 未知资源类型：仅保留通用键，避免臆造字段。
	}

	return attrs
}
