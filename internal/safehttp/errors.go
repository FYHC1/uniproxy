package safehttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
)

// ErrorKind is a stable, secret-free category for subscription download failures.
type ErrorKind string

const (
	KindURL            ErrorKind = "url"
	KindDNS            ErrorKind = "dns"
	KindPrivateAddress ErrorKind = "private_address"
	KindFakeIPFallback ErrorKind = "fake_ip_fallback"
	KindConnect        ErrorKind = "connect"
	KindConnectTimeout ErrorKind = "connect_timeout"
	KindTLS            ErrorKind = "tls"
	KindRedirect       ErrorKind = "redirect"
	KindHTTPStatus     ErrorKind = "http_status"
	KindBodyRead       ErrorKind = "body_read"
	KindBodyTooLarge   ErrorKind = "body_too_large"
	KindHTML           ErrorKind = "html"
	KindTimeout        ErrorKind = "timeout"
	KindCanceled       ErrorKind = "canceled"
	KindDownload       ErrorKind = "download"
)

// FetchError never retains an upstream error, URL, hostname, response body, or
// certificate detail. Its cause is another FetchError or a context sentinel.
// Callers may expose Error() and inspect Kind/StatusCode without leaking tokens.
type FetchError struct {
	Kind       ErrorKind
	StatusCode int
	cause      error
}

func (e *FetchError) Error() string {
	if e == nil {
		return "订阅下载失败"
	}
	switch e.Kind {
	case KindURL:
		return "订阅地址无效：必须使用不含用户信息和片段的 HTTP/HTTPS URL"
	case KindDNS:
		if errors.Is(e.cause, context.DeadlineExceeded) {
			return "订阅主机 DNS 解析超时"
		}
		return "订阅主机 DNS 解析失败"
	case KindPrivateAddress:
		return "默认禁止访问内网或保留地址订阅（包含混合的非公网 DNS 结果）"
	case KindFakeIPFallback:
		return "检测到 DNS Fake-IP，公共 DNS 安全回退失败（" + fallbackReason(e.cause) + "）"
	case KindConnect:
		return "订阅主机连接失败"
	case KindConnectTimeout:
		return "订阅主机连接超时"
	case KindTLS:
		if errors.Is(e.cause, context.DeadlineExceeded) {
			return "TLS 握手超时；未降低证书安全要求"
		}
		return "TLS 证书校验或握手失败；未降低证书安全要求"
	case KindRedirect:
		return "订阅重定向被拒绝：地址无效、次数过多或 HTTPS 降级"
	case KindHTTPStatus:
		if e.StatusCode >= 100 && e.StatusCode <= 999 {
			return fmt.Sprintf("订阅服务器返回 HTTP %d", e.StatusCode)
		}
		return "订阅服务器返回非成功 HTTP 状态"
	case KindBodyRead:
		if errors.Is(e.cause, context.DeadlineExceeded) {
			return "读取订阅响应超时"
		}
		return "读取订阅响应失败"
	case KindBodyTooLarge:
		return "订阅超过 5 MiB 限制"
	case KindHTML:
		return "订阅返回 HTML 页面而非配置（可能需要登录或被拦截）"
	case KindTimeout:
		return "订阅请求超时"
	case KindCanceled:
		return "订阅请求已取消"
	default:
		return "订阅下载失败（未知网络错误）"
	}
}

func (e *FetchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// ErrorKindOf returns the outer safe category, or the empty string for an error
// not created by this package. In particular a fallback failure stays identifiable
// even when its sanitized cause describes TLS, timeout, or a blocked DNS answer.
func ErrorKindOf(err error) ErrorKind {
	var e *FetchError
	if errors.As(err, &e) && e != nil {
		return e.Kind
	}
	return ""
}

func failure(kind ErrorKind) *FetchError { return &FetchError{Kind: kind} }

func contextFailure(err error) *FetchError {
	if errors.Is(err, context.Canceled) {
		return &FetchError{Kind: KindCanceled, cause: context.Canceled}
	}
	return &FetchError{Kind: KindTimeout, cause: context.DeadlineExceeded}
}

// sanitizeError inspects types only. It deliberately does not use err.Error(),
// string matching, or unwrap an upstream error into anything publicly retained.
func sanitizeError(ctx context.Context, err error, stage ErrorKind) *FetchError {
	if ctx != nil && ctx.Err() != nil {
		return contextFailure(ctx.Err())
	}
	var safe *FetchError
	if errors.As(err, &safe) && safe != nil {
		return safe
	}
	if errors.Is(err, context.Canceled) {
		return contextFailure(context.Canceled)
	}
	if isTLSError(err) {
		return failure(KindTLS)
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		kind := stage
		switch stage {
		case KindConnect:
			kind = KindConnectTimeout
		case KindDNS, KindTLS, KindBodyRead:
			// Keep the failure stage as well as errors.Is(DeadlineExceeded).
		default:
			kind = KindTimeout
		}
		return &FetchError{Kind: kind, cause: context.DeadlineExceeded}
	}
	return failure(stage)
}

func isTLSError(err error) bool {
	var verification *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var record tls.RecordHeaderError
	var alert tls.AlertError
	return errors.As(err, &verification) || errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostname) || errors.As(err, &invalid) ||
		errors.As(err, &record) || errors.As(err, &alert)
}

func fallbackReason(err error) string {
	switch ErrorKindOf(err) {
	case KindPrivateAddress:
		return "公共 DNS 返回内网或保留地址，已拒绝连接"
	case KindTLS:
		return "公共 DNS 的 TLS 校验或握手失败"
	case KindTimeout, KindConnectTimeout:
		return "公共 DNS 请求超时"
	case KindCanceled:
		return "公共 DNS 请求已取消"
	case KindConnect, KindDownload:
		return "无法连接公共 DNS 服务"
	case KindRedirect:
		return "公共 DNS 服务的重定向已拒绝"
	case KindHTTPStatus:
		var e *FetchError
		if errors.As(err, &e) && e.StatusCode >= 100 && e.StatusCode <= 999 {
			return fmt.Sprintf("公共 DNS 返回 HTTP %d", e.StatusCode)
		}
		return "公共 DNS 返回非成功 HTTP 状态"
	case KindBodyTooLarge:
		return "公共 DNS 响应超过安全大小限制"
	case KindBodyRead:
		return "无法读取公共 DNS 响应"
	default:
		return "公共 DNS 响应无效或没有可用公网地址"
	}
}
