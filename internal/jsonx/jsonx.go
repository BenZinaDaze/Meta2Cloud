// Package jsonx 提供与 Python dict.get 行为一致的动态 JSON 读取工具。
// TMDB 等接口返回结构灵活（电影/剧集字段不同），统一按 map[string]any 处理。
package jsonx

// Str 取字符串值，缺失或类型不符时返回空串。
func Str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// StrPtr 取字符串值，键不存在或为 null 时返回 nil。
func StrPtr(m map[string]any, key string) *string {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return &s
}

// Int 取整数值，缺失或类型不符时返回 fallback。
func Int(m map[string]any, key string, fallback int64) int64 {
	if m == nil {
		return fallback
	}
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return fallback
}

// Float 取浮点值，缺失或类型不符时返回 fallback。
func Float(m map[string]any, key string, fallback float64) float64 {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(float64); ok {
		return v
	}
	return fallback
}

// Bool 取布尔值，缺失时返回 fallback。
func Bool(m map[string]any, key string, fallback bool) bool {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return fallback
}

// Map 取子字典，缺失或类型不符时返回 nil。
func Map(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

// Slice 取子数组，缺失或类型不符时返回 nil。
func Slice(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

// MapSlice 取字典数组，元素非字典的项被跳过。
func MapSlice(m map[string]any, key string) []map[string]any {
	items := Slice(m, key)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if d, ok := item.(map[string]any); ok {
			out = append(out, d)
		}
	}
	return out
}
