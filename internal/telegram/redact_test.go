// notify_redact_test.go —— 错误日志脱敏（bot token 不随 *url.Error 外泄）测试。
package telegram

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURLError(t *testing.T) {
	inner := errors.New("context deadline exceeded (Client.Timeout exceeded)")
	ue := &url.Error{
		Op:  "Post",
		URL: "https://api.telegram.org/bot123456:ABC-SECRET/sendMessage",
		Err: inner,
	}
	got := redactURLError(ue)
	if strings.Contains(got.Error(), "123456:ABC-SECRET") {
		t.Errorf("脱敏后仍含 bot token: %v", got)
	}
	if !strings.Contains(got.Error(), "Client.Timeout") {
		t.Errorf("应保留底层错误信息: %v", got)
	}
	if got := redactURLError(inner); !errors.Is(got, inner) {
		t.Errorf("普通错误应原样返回: %v", got)
	}
}
