// jsonutil.go —— 与 Python dict.get 行为一致的动态 JSON 读取工具。
// 转调 internal/jsonx 消除双实现；仅保留 jsonx 未覆盖的指针语义变体。
package u115

import (
	"strconv"

	"panoply/internal/jsonx"
)

func jString(m map[string]any, key string) string { return jsonx.Str(m, key) }

// jStringAny 取字段值的字符串形式；字符串原样返回，数字/布尔按 Python str() 语义
// 格式化（整值不带小数点）。115 部分字段（如 authDeviceCode 的 time）会在
// 字符串与数字之间变动， strictly 类型断言会得到空串导致下游签名校验失败。
func jStringAny(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func jStringPtr(m map[string]any, key string) *string { return jsonx.StrPtr(m, key) }

func jNumber(m map[string]any, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key].(float64)
	return v, ok
}

// jIntPtr 取整数值，键不存在或为 null/空串时返回 nil。
func jIntPtr(m map[string]any, key string) *int64 {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil || v == "" {
		return nil
	}
	f, ok := v.(float64)
	if !ok {
		return nil
	}
	n := int64(f)
	return &n
}

func jInt(m map[string]any, key string, fallback int64) int64 {
	return jsonx.Int(m, key, fallback)
}

func jFloat(m map[string]any, key string, fallback float64) float64 {
	return jsonx.Float(m, key, fallback)
}

func jBool(m map[string]any, key string, fallback bool) bool {
	return jsonx.Bool(m, key, fallback)
}

func jMap(m map[string]any, key string) map[string]any { return jsonx.Map(m, key) }

func jSlice(m map[string]any, key string) []any { return jsonx.Slice(m, key) }

func jMapSlice(m map[string]any, key string) []map[string]any {
	return jsonx.MapSlice(m, key)
}

// strPtr 便捷构造字符串指针。
func strPtr(s string) *string { return &s }
