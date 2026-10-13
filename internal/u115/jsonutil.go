// jsonutil.go —— 与 Python dict.get 行为一致的动态 JSON 读取工具。
// 转调 internal/jsonx 消除双实现；仅保留 jsonx 未覆盖的指针语义变体。
package u115

import "meta2cloud/internal/jsonx"

func jString(m map[string]any, key string) string { return jsonx.Str(m, key) }

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
