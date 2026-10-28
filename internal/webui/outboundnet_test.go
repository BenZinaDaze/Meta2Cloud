// outboundnet_test.go —— 服务端外呼 URL 校验（SSRF 防护）测试。
// 用 IP 字面量与本地 hosts（localhost）覆盖各分支，不依赖外部 DNS。
package webui

import "testing"

func TestValidateOutboundURL(t *testing.T) {
	bad := []string{
		"ftp://example.com/a.torrent",             // 非 http/https
		"file:///etc/passwd",                      // 非 http/https
		"/relative/path",                          // 缺 scheme/主机
		"http://127.0.0.1:8080/a.torrent",         // 环回
		"http://localhost/a.torrent",              // 环回（hosts 解析）
		"http://[::1]/a.torrent",                  // 环回 IPv6
		"http://10.1.2.3/a.torrent",               // RFC1918
		"http://172.16.0.1/a.torrent",             // RFC1918
		"http://192.168.1.1/a.torrent",            // RFC1918
		"http://169.254.169.254/latest/meta-data", // 链路本地（云元数据）
		"http://0.0.0.0/a.torrent",                // 未指定地址
	}
	for _, u := range bad {
		if err := validateOutboundURL(u); err == nil {
			t.Errorf("validateOutboundURL(%q) 应拒绝", u)
		}
	}
	good := []string{
		"https://8.8.8.8/a.torrent",
		"http://1.1.1.1/download.torrent",
	}
	for _, u := range good {
		if err := validateOutboundURL(u); err != nil {
			t.Errorf("validateOutboundURL(%q) 应放行: %v", u, err)
		}
	}
}
