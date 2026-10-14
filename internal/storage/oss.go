// oss.go —— 最小化阿里云 OSS 分片上传实现（替代 Python 版 oss2 依赖）。
//
// 仅覆盖 115 上传所需的三个操作：
//   - InitiateMultipartUpload
//   - UploadPart（PUT）
//   - CompleteMultipartUpload（含 115 回调头）
//
// 签名采用 OSS V1（Authorization: OSS AK:Signature + x-oss-security-token）。
package storage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const ossPartPreferredSize = 10 * 1024 * 1024 // 与 Python 版 preferred_size 一致

type ossClient struct {
	endpoint string // 形如 http://oss-cn-shenzhen.aliyuncs.com
	bucket   string
	ak       string
	sk       string
	token    string
	hc       *http.Client
}

type ossPart struct {
	Number int
	ETag   string
}

// ossError OSS 错误响应。
type ossError struct {
	Code    string
	Message string
}

func (e *ossError) Error() string { return fmt.Sprintf("OSS %s: %s", e.Code, e.Message) }

type ossErrorXML struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// buildURL 虚拟主机风格：{scheme}://{bucket}.{host}/{object}。
func (o *ossClient) buildURL(object string, query url.Values) (string, error) {
	u, err := url.Parse(normalizeEndpoint(o.endpoint))
	if err != nil {
		return "", err
	}
	result := url.URL{
		Scheme:   u.Scheme,
		Host:     o.bucket + "." + u.Host,
		Path:     "/" + object,
		RawQuery: query.Encode(),
	}
	return result.String(), nil
}

// normalizeEndpoint 补全协议头。
func normalizeEndpoint(endpoint string) string {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return "https://" + endpoint
	}
	return endpoint
}

// canonicalHeaders 规范化 x-oss-* 头（小写、排序、k:v\n）。
func canonicalHeaders(h http.Header) string {
	var keys []string
	vals := map[string]string{}
	for k, v := range h {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-oss-") {
			keys = append(keys, lk)
			vals[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(":")
		b.WriteString(vals[k])
		b.WriteString("\n")
	}
	return b.String()
}

// ossSignSubresources 参与签名的子资源（OSS V1 规范节选）。
var ossSignSubresources = map[string]bool{
	"acl": true, "uploads": true, "partNumber": true, "uploadId": true,
	"security-token": true, "position": true, "callback": true, "callback-var": true,
	"encoding-type": true, "response-content-type": true, "response-content-language": true,
	"response-expires": true, "response-cache-control": true,
	"response-content-disposition": true, "response-content-encoding": true,
}

// signedResource 构造 /{bucket}/{object} + 参与签名的子资源。
func (o *ossClient) signedResource(object string, query url.Values) string {
	resource := "/" + o.bucket + "/" + object
	var sub []string
	for k, v := range query {
		if ossSignSubresources[k] {
			if len(v) > 0 && v[0] != "" {
				sub = append(sub, k+"="+v[0])
			} else {
				sub = append(sub, k)
			}
		}
	}
	sort.Strings(sub)
	if len(sub) > 0 {
		resource += "?" + strings.Join(sub, "&")
	}
	return resource
}

// sign 构造签名。resource 为 signedResource 的返回值。
func (o *ossClient) sign(verb, contentType, date string, headers http.Header, resource string) string {
	stringToSign := verb + "\n" + "" + "\n" + contentType + "\n" + date + "\n" + canonicalHeaders(headers) + resource
	mac := hmac.New(sha1.New, []byte(o.sk))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// doOSS 执行一次签名请求。respHeaders 可为 nil；out 非 nil 时解析 XML 响应。
func (o *ossClient) doOSS(verb, object string, query url.Values, headers http.Header, body []byte, out any, respHeaders http.Header) error {
	if headers == nil {
		headers = http.Header{}
	}
	if o.token != "" {
		headers.Set("x-oss-security-token", o.token)
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	headers.Set("Date", date)
	sig := o.sign(verb, headers.Get("Content-Type"), date, headers, o.signedResource(object, query))
	headers.Set("Authorization", "OSS "+o.ak+":"+sig)

	target, err := o.buildURL(object, query)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(verb, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	resp, err := o.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if respHeaders != nil {
		for k, v := range resp.Header {
			respHeaders[k] = v
		}
	}
	if resp.StatusCode >= 300 {
		var exml ossErrorXML
		if xmlErr := xml.Unmarshal(raw, &exml); xmlErr == nil && exml.Code != "" {
			return &ossError{Code: exml.Code, Message: exml.Message}
		}
		return fmt.Errorf("OSS HTTP %d: %s", resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		return xml.Unmarshal(raw, out)
	}
	return nil
}

type ossInitiateResult struct {
	UploadID string `xml:"UploadId"`
}

func (o *ossClient) initiateMultipartUpload(object string) (string, error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/octet-stream")
	var result ossInitiateResult
	if err := o.doOSS(http.MethodPost, object, url.Values{"uploads": {""}}, headers, nil, &result, nil); err != nil {
		return "", err
	}
	return result.UploadID, nil
}

func (o *ossClient) uploadPart(object, uploadID string, partNumber int, data []byte) (string, error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/octet-stream")
	query := url.Values{
		"partNumber": {strconv.Itoa(partNumber)},
		"uploadId":   {uploadID},
	}
	respHeaders := http.Header{}
	if err := o.doOSS(http.MethodPut, object, query, headers, data, nil, respHeaders); err != nil {
		return "", err
	}
	return strings.Trim(respHeaders.Get("ETag"), `"`), nil
}

func (o *ossClient) completeMultipartUpload(object, uploadID string, parts []ossPart, callbackBody, callbackVar string) error {
	type xmlPart struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	}
	body, err := xml.Marshal(struct {
		XMLName xml.Name  `xml:"CompleteMultipartUpload"`
		Parts   []xmlPart `xml:"Part"`
	}{Parts: func() []xmlPart {
		out := make([]xmlPart, 0, len(parts))
		for _, p := range parts {
			out = append(out, xmlPart{PartNumber: p.Number, ETag: p.ETag})
		}
		return out
	}()})
	if err != nil {
		return err
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/xml")
	headers.Set("x-oss-forbid-overwrite", "false")
	if callbackBody != "" {
		headers.Set("x-oss-callback", base64.StdEncoding.EncodeToString([]byte(callbackBody)))
	}
	if callbackVar != "" {
		headers.Set("x-oss-callback-var", base64.StdEncoding.EncodeToString([]byte(callbackVar)))
	}
	err = o.doOSS(http.MethodPost, object, url.Values{"uploadId": {uploadID}}, headers, body, nil, nil)
	if err != nil {
		// 目标已存在视为成功（与 Python 版一致）
		if oe, ok := err.(*ossError); ok && oe.Code == "FileAlreadyExists" {
			return nil
		}
		return err
	}
	return nil
}
