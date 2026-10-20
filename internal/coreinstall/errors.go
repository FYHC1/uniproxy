package coreinstall

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/uniproxy/uniproxy/internal/safehttp"
)

var (
	errUntrustedURL = errors.New("安装来源不可信，已拒绝请求；只允许 GitHub 官方 HTTPS 发布地址")
	errRedirect     = errors.New("安装包重定向不安全或次数过多，已拒绝下载；请检查网络拦截后重试")
	errMetadata     = errors.New("GitHub 固定版本发布信息无效或不完整；请稍后重试，不要使用未经校验的安装包")
	errMetadataPin  = errors.New("官方安装包 SHA256 与内置固定摘要不一致，已拒绝下载；请稍后重试或联系维护者核实，勿绕过校验")
	errChecksum     = errors.New("安装包 SHA256 校验失败，未安装内核；请检查网络或磁盘后重试，勿绕过校验")
	errDownloadSize = errors.New("安装包下载不完整或大小超过安全限制（最多 128 MiB），未安装内核；请检查服务器网络后重试")
	errArchive      = errors.New("已校验安装包的归档结构无效或不安全，未安装内核；请重试，若仍失败请联系维护者核实")
	errBinarySize   = errors.New("解压出的内核为空或超过 256 MiB 安全限制，未安装内核；请联系维护者核实安装包")
)

// Only safe sentinels are retained as causes. Raw URL, HTTP body, certificate,
// filesystem path and transport errors must not reach the API or its logs.
type installError struct {
	text  string
	cause error
}

func (e *installError) Error() string { return e.text }
func (e *installError) Unwrap() error { return e.cause }

func checkContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return contextFailure(err)
	}
	return nil
}

func contextFailure(err error) error {
	if errors.Is(err, context.Canceled) {
		return &installError{text: "内核安装已取消；需要时可重新发起安装", cause: context.Canceled}
	}
	return &installError{text: "内核安装超时（单次安装最长 5 分钟）；请检查服务器网络后重试", cause: context.DeadlineExceeded}
}

func networkFailure(ctx context.Context, err error) error {
	if failure := checkContext(ctx); failure != nil {
		return failure
	}
	if errors.Is(err, context.Canceled) {
		return contextFailure(context.Canceled)
	}
	if errors.Is(err, errUntrustedURL) {
		return errUntrustedURL
	}
	if errors.Is(err, errRedirect) {
		return errRedirect
	}
	switch safehttp.ErrorKindOf(err) {
	case safehttp.KindURL:
		return errUntrustedURL
	case safehttp.KindRedirect:
		return errRedirect
	case safehttp.KindDNS:
		return errors.New("GitHub 主机 DNS 解析失败或超时；请检查服务器 DNS 后重试")
	case safehttp.KindPrivateAddress:
		return errors.New("GitHub 解析到了内网或保留地址，已拒绝连接；请检查服务器 DNS，勿关闭安全校验")
	case safehttp.KindFakeIPFallback:
		return errors.New("检测到 DNS Fake-IP，但公共 DNS 安全复核失败；请检查服务器能否访问公共 DNS 与 GitHub 后重试")
	case safehttp.KindTLS:
		return errors.New("GitHub TLS 证书校验或握手失败；请检查系统时间和网络拦截后重试，勿关闭证书校验")
	case safehttp.KindConnect:
		return errors.New("无法连接 GitHub；请检查 UniProxy 所在服务器的直连网络后重试（不使用浏览器或系统 HTTP 代理）")
	case safehttp.KindCanceled:
		return contextFailure(context.Canceled)
	case safehttp.KindTimeout, safehttp.KindConnectTimeout:
		return contextFailure(context.DeadlineExceeded)
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return contextFailure(context.DeadlineExceeded)
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return errors.New("GitHub 主机 DNS 解析失败；请检查服务器 DNS 后重试")
	}
	return errors.New("GitHub 安装包网络下载失败；请检查 UniProxy 所在服务器能否直连 GitHub 后重试（不使用浏览器或系统 HTTP 代理）")
}

func httpFailure(code int) error {
	if code < 100 || code > 599 {
		return errors.New("GitHub 返回了无效的 HTTP 状态；请检查服务器网络后重试")
	}
	switch code {
	case 403, 429:
		return fmt.Errorf("GitHub 返回 HTTP %d（访问受限或请求过于频繁）；请稍后重试，并检查服务器能否直连 GitHub", code)
	case 404:
		return errors.New("GitHub 返回 HTTP 404，固定版本安装包暂不可用；请稍后重试或联系维护者核实")
	default:
		return fmt.Errorf("GitHub 返回 HTTP %d；请检查服务器网络或稍后重试", code)
	}
}

func destinationExists() error {
	return &installError{text: "内核目标文件已存在（可能由另一安装任务创建），已拒绝覆盖；请检查内核路径，不要覆盖正在使用的内核", cause: os.ErrExist}
}

func filesystemFailure(operation string, err error) error {
	if errors.Is(err, os.ErrPermission) {
		return &installError{text: "无法访问或写入内核安装目录（权限不足）；请为运行 UniProxy 的账户授予该目录访问和写入权限后重试", cause: os.ErrPermission}
	}
	// operation is always a fixed string owned by this package, never a path.
	return fmt.Errorf("%s失败；请检查安装目录权限、磁盘空间或安全软件后重试", operation)
}

func publicationFailure(err error) error {
	if errors.Is(err, os.ErrExist) {
		return destinationExists()
	}
	if errors.Is(err, os.ErrPermission) {
		return filesystemFailure("发布已验证内核", err)
	}
	return errors.New("无法以不覆盖方式安装内核；请检查目录权限并使用支持硬链接的文件系统后重试（不会降级为覆盖安装）")
}
