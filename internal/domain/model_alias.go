package domain

import "strings"

// KnownCloudProviders 是 e-cam-service 多云同步可能写入的 model_uid 厂商前缀全集
// （其同步执行器统一以 fmt.Sprintf("%s_<类型>", account.Provider) 拼接 model_uid）。
//
// 来源：e-cam internal/cam/domain/account.go 的 CloudProvider 枚举
// （aliyun/aws/azure/tencent/huawei），外加实例数据中实际观察到的
// volcengine/volcano（火山引擎账号在 e-cam 侧 provider 为 volcengine，
// 而本服务模型族前缀为 volcano，见 providerBridge）。
var KnownCloudProviders = []string{
	"aliyun", "aws", "azure", "tencent", "huawei", "volcengine", "volcano",
}

// providerBridge 桥接「e-cam 账号 Provider 名 → 本服务模型族前缀」的不一致命名。
// 目前唯一已知：火山引擎在 e-cam 侧为 volcengine，在本服务模型表为 volcano_*。
var providerBridge = map[string]string{
	"volcengine": "volcano",
}

// BuildModelAliases 依据模型全集生成「别名 → 模型 UID」归一映射，
// 供 e-cam-service 的 modelalias.RemoteResolver 拉取（实例写入前把
// {provider}_{类型} 形式的 model_uid 归一到本服务的真实模型 UID，
// 从源头消除孤儿实例）。
//
// 生成规则（优先级从低到高）：
//  1. 通用回退：对每个通用模型（Provider=="all"，如 snapshot/disk/lb/waf）
//     与每个已知厂商前缀 P，若 "P_UID" 不是真实模型，生成别名 P_UID → UID
//     （如 aliyun_snapshot → snapshot）。
//  2. 厂商族桥接：对每个模型族前缀不一致的厂商（providerBridge，当前为
//     volcengine→volcano），若 "volcano_T" 是真实模型，生成
//     volcengine_T → volcano_T（如 volcengine_ecs → volcano_ecs），
//     覆盖规则 1 的通用回退——族模型比通用模型更精确。
//
// 已是真实模型的 uid（如 aliyun_vpc、tencent_ecs）不生成别名：
// e-cam 写入时精确命中，无需归一。无对应目标的别名（如 *_vswitch，
// 模型表既无族模型也无裸名模型）不生成——那是模型表缺口，应由补模型
// 解决，别名层不掩盖。
func BuildModelAliases(models []Model) map[string]string {
	set := make(map[string]struct{}, len(models))
	for _, m := range models {
		set[m.UID] = struct{}{}
	}

	aliases := make(map[string]string)

	// 规则 1：通用回退
	for _, m := range models {
		if m.Provider != "all" {
			continue
		}
		for _, p := range KnownCloudProviders {
			alias := p + "_" + m.UID
			if _, ok := set[alias]; !ok {
				aliases[alias] = m.UID
			}
		}
	}

	// 规则 2：厂商族桥接（覆盖通用回退，族模型更精确）
	for aliasPrefix, familyPrefix := range providerBridge {
		family := familyPrefix + "_"
		for uid := range set {
			rest, ok := strings.CutPrefix(uid, family)
			if !ok {
				continue
			}
			alias := aliasPrefix + "_" + rest
			if _, exists := set[alias]; !exists {
				aliases[alias] = uid
			}
		}
	}

	return aliases
}
