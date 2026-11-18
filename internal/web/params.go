package web

import (
	"fmt"
	"strconv"
	"strings"
)

// 非分页的数值/枚举查询参数校验。
//
// 背景（第228轮）：/cmdb/topology/instance/:id、/cmdb/impact、/cmdb/models 同样用
// `strconv.Atoi(...)` 后丢弃错误。它们不会像分页那样「吐全表」，但会静默改成另一个
// 语义，比报错更难发现：
//
//   - topology 的 depth 用 `query.Depth > 0` 作限深判断（internal/service/topology.go:82），
//     即 depth=0 表示**不限深度**。于是 depth=abc、depth=-3 这类错值反而把遍历从
//     「1 层」放大成「整个连通分量」（实测云盘节点从 2 变成 4，且随数据增长无上界）。
//   - impact 的 depth 在 service 里 `depth<1` 回落 1（internal/service/impact.go:54），
//     错值被当成 1，用户以为改了范围其实没改。
//   - models 的 level / model_group_id 解析失败即 0，而 0 表示「不过滤」
//     （dao 的过滤分支不触发），等于静默丢弃筛选条件（实测 level=abc → total 81，
//     level=1 → total 41）。
//   - topology 的 direction 不在白名单时两个遍历分支都不进入，返回只有根节点的空图。
//
// 统一约定：非法值返回 400 + code=ParamsError + 中文原因；空值沿用各自默认值。

// maxGraphDepth 是拓扑/影响分析允许的最大层数。
//
// 前端实际取值：拓扑页 1..3 层（views/cmdb/topology/index.vue），影响分析 1..10
// （views/impact/index.vue 的 el-input-number :max="10"）。上限取 10 同时覆盖两者，
// 避免一次请求把遍历深度开到任意大。
const maxGraphDepth = 10

// parseBoundedInt 解析 [minVal, maxVal] 范围内的整数。
//
// raw 为空白时返回 defaultVal；非整数或越界时返回中文错误，调用方转成
// `ErrorResultWithMsg(errs.ParamsError, errMsg)`。
func parseBoundedInt(raw, name string, defaultVal, minVal, maxVal int) (int, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultVal, ""
	}
	v, err := strconv.Atoi(trimmed)
	if err != nil || v < minVal || v > maxVal {
		return defaultVal, fmt.Sprintf("%s 必须是 %d 到 %d 之间的整数（收到 %q）", name, minVal, maxVal, trimmed)
	}
	return v, ""
}

// parseNonNegativeInt64 解析非负整数，空白时返回 0（0 在各过滤器里表示「不过滤」）。
func parseNonNegativeInt64(raw, name string) (int64, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, ""
	}
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || v < 0 {
		return 0, fmt.Sprintf("%s 必须是不小于 0 的整数（收到 %q）", name, trimmed)
	}
	return v, ""
}

// topologyDirections 是拓扑遍历方向的全集。
//
// internal/service/topology.go:87 与 :134 各自判断一个分支，未知取值会让两个分支
// 都不进入、返回只有根节点的空图——因此必须在此收敛成白名单。
var topologyDirections = map[string]struct{}{
	"both":     {},
	"outgoing": {},
	"incoming": {},
}

// parseTopologyDirection 校验方向参数，空白回落 both。
func parseTopologyDirection(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "both", ""
	}
	if _, ok := topologyDirections[trimmed]; ok {
		return trimmed, ""
	}
	return "both", fmt.Sprintf("direction 只能是 both、outgoing 或 incoming（收到 %q）", trimmed)
}
