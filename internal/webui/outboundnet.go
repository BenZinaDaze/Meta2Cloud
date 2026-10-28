// outboundnet.go —— 服务端代为请求外部 URL 的安全校验（SSRF 防护）。
package webui

import (
	"fmt"
	"net"
	"net/url"
)

// validateOutboundURL 校验服务端将直接请求的 URL：仅允许 http/https，
// 且目标主机的全部解析 IP 均不得为环回/私网/链路本地/未指定地址。
// 用于种子链接下载等 URL 可能源自外部内容（RSS 条目）的场合，
// 防止借服务端之手探测内网、读取内网资源并外带。
func validateOutboundURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 无效：%v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https 链接")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}
	if ip := net.ParseIP(host); ip != nil {
		return checkPublicIP(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("主机名无法解析：%v", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("主机名未解析出任何地址")
	}
	for _, ip := range ips {
		if err := checkPublicIP(ip); err != nil {
			return err
		}
	}
	return nil
}

// checkPublicIP 拒绝不可作为外呼目标的地址段。
func checkPublicIP(ip net.IP) error {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("不允许访问内网/本机地址（%s）", ip)
	}
	return nil
}
