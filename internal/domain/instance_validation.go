package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FieldIssue 一条字段级校验结果。
//
// 设计背景（2026-11-02）：
// 后端此前**完全不校验模型字段** —— Instance.Validate() 只检查 model_uid/asset_id/tenant_id
// 三个通用字段，对模型定义的 required / enum / field_type 一律不校验。
// 实测后果：4132 条实例缺失模型定义的必填字段，且前端表单也不校验，
// 无效数据可以随意写入。
//
// 按产品已确认的规则实现：
//   - 必填缺失 → **错误**（新建时硬拦；更新时不拦，见 ValidateAttributes 的说明）
//   - 枚举不匹配 → **仅警告**（且大小写不敏感：数据里 `running` 与定义 `Running` 视为匹配）
//   - 类型不匹配 → **仅警告**
type FieldIssue struct {
	Field   string `json:"field"`
	Type    string `json:"type"`    // required / enum / type
	Message string `json:"message"` // 面向用户的中文说明
}

// ValidationResult 字段校验汇总
type ValidationResult struct {
	Errors   []FieldIssue
	Warnings []FieldIssue
}

// HasErrors 是否存在阻断性错误
func (r ValidationResult) HasErrors() bool { return len(r.Errors) > 0 }

// Error 把阻断性错误拼成一条可读消息（供 API 直接返回给用户）
func (r ValidationResult) Error() string {
	if !r.HasErrors() {
		return ""
	}
	parts := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, "；")
}

// ValidateAttributes 按模型字段定义校验实例属性。
//
// blockRequired 控制必填缺失是否算错误：
//   - 新建（blockRequired=true）：必填缺失 → Errors，硬拦
//   - 更新（blockRequired=false）：必填缺失 → Warnings，不拦
//
// 为什么更新不硬拦：存量数据 4132 条已缺失必填字段（如 ecs 的 instance_id/instance_name
// 在真实数据里从不存在，数据用 asset_id/asset_name）。若更新也硬拦，
// 这些历史实例将**无法再被编辑**，属明显回归。产品已确认「只对新写入生效」。
//
// 枚举与类型问题一律只进 Warnings，不阻断写入。
func ValidateAttributes(attrs map[string]interface{}, defs []Attribute, blockRequired bool) ValidationResult {
	var res ValidationResult
	for _, def := range defs {
		if def.FieldUID == "" {
			continue
		}
		raw, exists := attrs[def.FieldUID]
		empty := !exists || raw == nil || strings.TrimSpace(fmt.Sprintf("%v", raw)) == ""

		if empty {
			if def.Required {
				issue := FieldIssue{
					Field:   def.FieldUID,
					Type:    "required",
					Message: fmt.Sprintf("缺少必填字段 %s", def.FieldUID),
				}
				if blockRequired {
					res.Errors = append(res.Errors, issue)
				} else {
					res.Warnings = append(res.Warnings, issue)
				}
			}
			continue
		}

		switch strings.ToLower(def.FieldType) {
		case "enum":
			if !enumMatches(def.Option, raw) {
				res.Warnings = append(res.Warnings, FieldIssue{
					Field:   def.FieldUID,
					Type:    "enum",
					Message: fmt.Sprintf("字段 %s 的值 %v 不在枚举范围 %v 内", def.FieldUID, raw, enumOptions(def.Option)),
				})
			}
		case "int", "float":
			if _, ok := toFloat(raw); !ok {
				res.Warnings = append(res.Warnings, FieldIssue{
					Field:   def.FieldUID,
					Type:    "type",
					Message: fmt.Sprintf("字段 %s 应为数值，实际为 %v", def.FieldUID, raw),
				})
			}
		case "bool":
			if _, ok := toBool(raw); !ok {
				res.Warnings = append(res.Warnings, FieldIssue{
					Field:   def.FieldUID,
					Type:    "type",
					Message: fmt.Sprintf("字段 %s 应为布尔值，实际为 %v", def.FieldUID, raw),
				})
			}
		}
	}
	return res
}

// enumOptions 解析枚举可选值。
//
// 实测（2026-11-02）：数据库中 Option 存的是 **JSON 字符串**，形如
// `"[\"aliyun\",\"aws\",\"tencent\"]"`（不是数组）。为兼容历史写入方式，
// 这里同时接受：[]interface{}、[]string、JSON 字符串、以及单个标量。
func enumOptions(option interface{}) []string {
	switch v := option.(type) {
	case nil:
		return nil
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, scalarToString(item))
		}
		return out
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		var arr []interface{}
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			out := make([]string, 0, len(arr))
			for _, item := range arr {
				out = append(out, scalarToString(item))
			}
			return out
		}
		return []string{s}
	default:
		return []string{scalarToString(v)}
	}
}

// enumMatches 判断值是否在枚举内。**大小写不敏感** —— 实测数据里
// status 为 `running`，而模型定义写的是 `Running`，严格比较会把全部存量数据判为非法。
func enumMatches(option, raw interface{}) bool {
	opts := enumOptions(option)
	if len(opts) == 0 {
		return true // 未定义可选值时不判定
	}
	target := strings.ToLower(strings.TrimSpace(scalarToString(raw)))
	for _, o := range opts {
		if strings.ToLower(strings.TrimSpace(o)) == target {
			return true
		}
	}
	return false
}

// scalarToString 把枚举项/属性值统一转成字符串
func scalarToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		// JSON 数字统一为 float64，避免 1 输出成 1.000000
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toBool(v interface{}) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		if s == "true" || s == "1" {
			return true, true
		}
		if s == "false" || s == "0" {
			return false, true
		}
		return false, false
	default:
		return false, false
	}
}
