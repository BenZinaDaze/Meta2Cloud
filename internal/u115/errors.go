package u115

import (
	"errors"
	"fmt"
)

// ErrorKind 错误类别，对应 Python 版的异常继承层级。
type ErrorKind int

const (
	KindAPI       ErrorKind = iota // 普通业务错误（Pan115ApiError）
	KindRateLimit                  // 命中风控或限流（Pan115RateLimitError）
	KindAuth                       // 认证或令牌异常（Pan115AuthError）
)

// APIError 115 接口异常。
// 通过 Kind 区分限流 / 认证失败 / 普通业务错误。
type APIError struct {
	Kind    ErrorKind
	Message string
	Code    *float64 // 115 返回的业务码（JSON 数字，可能缺失）
	State   *bool
	Payload map[string]any
}

func (e *APIError) Error() string {
	if e.Code != nil {
		return fmt.Sprintf("%s (code=%d)", e.Message, int64(*e.Code))
	}
	return e.Message
}

// NewAPIError 构造普通业务错误。
func NewAPIError(message string) *APIError { return &APIError{Kind: KindAPI, Message: message} }

// IsRateLimit 判断是否为限流错误。
func IsRateLimit(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Kind == KindRateLimit
}

// IsAuth 判断是否为认证错误。
func IsAuth(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Kind == KindAuth
}

// CodeValue 返回业务码（缺失时返回 0）。
func (e *APIError) CodeValue() int64 {
	if e.Code == nil {
		return 0
	}
	return int64(*e.Code)
}
