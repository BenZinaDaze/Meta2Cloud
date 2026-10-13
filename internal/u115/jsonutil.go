package u115

// 115 接口返回大量动态 JSON，这里提供与 Python dict.get 行为一致的小工具。

// jString 取字符串值，缺失或类型不符时返回空串。
func jString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// jStringPtr 取字符串值，键不存在或为 null 时返回 nil。
func jStringPtr(m map[string]any, key string) *string {
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

// jNumber 取数字值（JSON 数字统一为 float64），缺失或类型不符时返回 (0, false)。
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

// jInt 取整数值，缺失或类型不符时返回 fallback。
func jInt(m map[string]any, key string, fallback int64) int64 {
	if n := jIntPtr(m, key); n != nil {
		return *n
	}
	return fallback
}

// jFloat 取浮点值，缺失或类型不符时返回 fallback。
func jFloat(m map[string]any, key string, fallback float64) float64 {
	if f, ok := jNumber(m, key); ok {
		return f
	}
	return fallback
}

// jBool 取布尔值，缺失时返回 fallback。
func jBool(m map[string]any, key string, fallback bool) bool {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return fallback
}

// jMap 取子字典，缺失或类型不符时返回 nil。
func jMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

// jSlice 取子数组，缺失或类型不符时返回 nil。
func jSlice(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

// jMapSlice 取字典数组，元素非字典的项被跳过。
func jMapSlice(m map[string]any, key string) []map[string]any {
	items := jSlice(m, key)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if d, ok := item.(map[string]any); ok {
			out = append(out, d)
		}
	}
	return out
}

// strPtr 便捷构造字符串指针。
func strPtr(s string) *string { return &s }
