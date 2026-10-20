package app

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/uniproxy/uniproxy/internal/coreinstall"
	"github.com/uniproxy/uniproxy/internal/model"
)

// Optional to keep non-installable adapters and isolated test cores usable. The
// production Manager owns destination discovery, reservation and process guards.
type installableCore interface {
	InstallationStatus() model.CoreInstallation
	PrepareInstall(context.Context) (model.CoreInstallation, error)
	RunInstall(context.Context) error
}

func (a *App) coreInstallation() model.CoreInstallation {
	if c, ok := a.core.(installableCore); ok {
		s := c.InstallationStatus()
		if a.installPending && s.State != "installing" {
			s.State, s.Stage = "installing", "inspecting"
			s.CanInstall, s.Reason = false, "正在完成安装检查，请稍候"
		}
		if a.closed {
			s.CanInstall, s.Reason = false, "管理服务正在关闭"
		}
		return s
	}
	return model.CoreInstallation{
		State: "idle", Version: coreinstall.PinnedVersion, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Installed: a.core.Status().Available, Reason: "当前内核适配器不支持在线安装",
	}
}

func (a *App) installationStatus() model.CoreInstallation {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.coreInstallation()
}

func emptyCoreRequest(w http.ResponseWriter, r *http.Request) error {
	var body *struct{}
	if err := decode(w, r, &body); err != nil {
		return err
	}
	if body == nil {
		return bad("请求必须为空 JSON 对象 {}，不能指定下载地址、版本或目标路径")
	}
	return nil
}

func (a *App) startCoreInstall(ctx context.Context) (model.CoreInstallation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.background.Err() != nil {
		return model.CoreInstallation{}, &apiError{503, "管理服务正在关闭，无法安装内核", "shutting_down"}
	}
	if ctx.Err() != nil {
		return model.CoreInstallation{}, &apiError{408, "安装请求已取消，请重新确认状态", "request_cancelled"}
	}
	if a.installPending {
		return model.CoreInstallation{}, &apiError{409, "已有内核安装任务，请查看安装进度", "core_install_busy"}
	}
	c, ok := a.core.(installableCore)
	if !ok {
		return model.CoreInstallation{}, &apiError{409, "当前内核适配器不支持在线安装", "core_install_unavailable"}
	}
	s, err := c.PrepareInstall(ctx)
	if err != nil {
		return model.CoreInstallation{}, &apiError{409, err.Error(), "core_install_unavailable"}
	}
	a.installPending = true
	// A configuration mutation (including scheduled subscriptions) must not start
	// proxy traffic as a side effect of finishing this installation. Apply is an
	// explicit, separate administrative action; manual-stop semantics are retained.
	a.awaitingCoreApply = true
	a.applyError = "Mihomo 正在安装；配置仅保存，安装完成后请手动应用配置"
	a.store.Activity("info", "开始在线安装固定版本 Mihomo v"+coreinstall.PinnedVersion)
	a.wg.Add(1)
	go a.runCoreInstall(c)
	return s, nil
}

func (a *App) runCoreInstall(c installableCore) {
	defer a.wg.Done()
	ctx, cancel := context.WithTimeout(a.background, 5*time.Minute)
	defer cancel()
	err := c.RunInstall(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.installPending = false
	if err != nil {
		message := err.Error()
		if errors.Is(err, context.Canceled) || a.background.Err() != nil {
			message = "管理服务关闭，安装已取消；下次启动可重新尝试"
		}
		a.applyError = "Mihomo 安装失败：" + message
		a.store.Activity("warning", a.applyError)
		return
	}
	a.applyError = "Mihomo 已安装并通过版本检测；点击重新应用配置后启动代理"
	a.store.Activity("info", "Mihomo 内核已安装，等待管理员应用配置；尚未启动代理")
}

func (a *App) inspectCore(ctx context.Context) (any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.background.Err() != nil {
		return nil, &apiError{503, "管理服务正在关闭", "shutting_down"}
	}
	if a.installPending {
		return nil, &apiError{409, "内核正在安装，请等待完成后重新检测", "core_install_busy"}
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	r := a.core.Inspect(inspectCtx)
	if !r.Available {
		a.applyError = "内核尚不可用，请在线安装或检查内核文件后重新检测"
		if r.LastError != "" {
			a.applyError += "：" + r.LastError
		}
	} else if !r.Running {
		a.applyError = "已检测到可用内核；点击重新应用配置后启动代理"
	}
	return map[string]any{"coreInstallation": a.coreInstallation(), "runtime": a.runtime()}, nil
}
