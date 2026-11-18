package web

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// defaultListLimit 与改造前的默认页大小保持一致，避免前端无参数时行为变化。
	defaultListLimit = 20
	// maxListLimit 覆盖前端现有最大取数（服务树绑定弹窗按 500 拉候选资源）。
	maxListLimit = 500
)

// resolvePaging 解析并校验列表分页参数。
//
// 改造前 handler 用 `strconv.Atoi(...)` 后忽略错误，配合 DAO「Limit<=0 表示不限」的语义，
// 任何非法值都会退化成**全量返回**：
//   - `?limit=0`、`?limit=-1`、`?limit=abc`、`?limit= ` → limit=0 → 一次返回 21,192 条 / 27.5 MB / 5.8s
//   - `?offset=-5` 被当成 0
//
// 这既是资源放大（单请求 27 MB、可按并发放大）也是隐蔽的语义错误（调用方以为分了页）。
// 这里改为显式校验并返回中文原因，交由 handler 以 400 返回，不再静默降级。
func resolvePaging(offsetRaw, limitRaw string) (offset, limit int, errMsg string) {
	if raw := strings.TrimSpace(offsetRaw); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return 0, 0, fmt.Sprintf("offset 必须是不小于 0 的整数（收到 %q）", raw)
		}
		offset = v
	}

	limit = defaultListLimit
	if raw := strings.TrimSpace(limitRaw); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxListLimit {
			return 0, 0, fmt.Sprintf("limit 必须是 1 到 %d 之间的整数（收到 %q）", maxListLimit, raw)
		}
		limit = v
	}

	return offset, limit, ""
}
