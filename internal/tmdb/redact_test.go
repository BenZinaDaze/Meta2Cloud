// redact_test.go —— 错误日志脱敏（api_key 不随 *url.Error 外泄）测试。
package tmdb

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURLError(t *testing.T) {
	inner := errors.New("dial tcp: lookup api.themoviedb.org: no such host")
	ue := &url.Error{
		Op:  "Get",
		URL: "https://api.themoviedb.org/3/search/movie?api_key=SECRET_KEY&query=x",
		Err: inner,
	}
	got := redactURLError(ue)
	if strings.Contains(got.Error(), "SECRET_KEY") {
		t.Errorf("脱敏后仍含 api_key: %v", got)
	}
	if !strings.Contains(got.Error(), "no such host") {
		t.Errorf("应保留底层错误信息: %v", got)
	}
	// 非 url.Error 的普通错误原样返回
	if got := redactURLError(inner); !errors.Is(got, inner) {
		t.Errorf("普通错误应原样返回: %v", got)
	}
}
