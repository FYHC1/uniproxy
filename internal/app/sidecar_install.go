package app

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/sidecarinstall"
)

// installableSidecar is optional so lightweight adapters and tests remain
// valid. The production XrayManager owns destination discovery, reservation
// and version verification of the fixed release.
type installableSidecar interface {
	InstallationStatus() model.CoreInstallation
	PrepareInstall(context.Context) (model.CoreInstallation, error)
	RunInstall(context.Context) error
}

func (a *App) sidecarInstallationLocked() model.CoreInstallation {
	if s, ok := a.sidecar.(installableSidecar); ok {
		installation := s.InstallationStatus()
		if a.sidecarInstallPending && installation.State != "installing" {
			installation.State, installation.Stage = "installing", "inspecting"
			installation.CanInstall, installation.Reason = false, "正在完成安装检查，请稍候"
		}
		if a.closed {
			installation.CanInstall, installation.Reason = false, "管理服务正在关闭"
		}
		return installation
	}
	return model.CoreInstallation{
		State: "idle", Version: sidecarinstall.PinnedVersion, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Reason: "当前 sidecar 适配器不支持在线安装",
	}
}

func (a *App) startSidecarInstall(ctx context.Context) (model.CoreInstallation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.background.Err() != nil {
		return model.CoreInstallation{}, &apiError{503, "管理服务正在关闭，无法安装 Xray", "shutting_down"}
	}
	if ctx.Err() != nil {
		return model.CoreInstallation{}, &apiError{408, "安装请求已取消，请重新确认状态", "request_cancelled"}
	}
	if a.installPending || a.sidecarInstallPending {
		return model.CoreInstallation{}, &apiError{409, "已有内核安装任务，请查看安装进度", "core_install_busy"}
	}
	installer, ok := a.sidecar.(installableSidecar)
	if !ok {
		return model.CoreInstallation{}, &apiError{409, "当前 sidecar 适配器不支持在线安装", "sidecar_install_unavailable"}
	}
	status, err := installer.PrepareInstall(ctx)
	if err != nil {
		return model.CoreInstallation{}, &apiError{409, err.Error(), "sidecar_install_unavailable"}
	}
	a.sidecarInstallPending = true
	// Configuration changes made while Xray downloads are saved only. A separate
	// explicit apply reconciles them after the binary is verified.
	a.awaitingCoreApply = true
	a.applyError = "Xray 正在安装；配置仅保存，安装完成后请手动应用配置"
	a.store.Activity("info", "开始在线安装固定版本 Xray v"+sidecarinstall.PinnedVersion)
	a.wg.Add(1)
	go a.runSidecarInstall(installer)
	return status, nil
}

func (a *App) runSidecarInstall(installer installableSidecar) {
	defer a.wg.Done()
	ctx, cancel := context.WithTimeout(a.background, 6*time.Minute)
	defer cancel()
	err := installer.RunInstall(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sidecarInstallPending = false
	if err != nil {
		message := err.Error()
		if errors.Is(err, context.Canceled) || a.background.Err() != nil {
			message = "管理服务关闭，安装已取消；下次启动可重新尝试"
		}
		a.applyError = "Xray 安装失败：" + message
		a.store.Activity("warning", a.applyError)
		return
	}
	a.applyError = "Xray 已安装并通过版本检测；点击重新应用配置后启动 Xray sidecar"
	a.store.Activity("info", "Xray sidecar 已安装，等待管理员应用配置；尚未启动代理")
}

func emptySidecarInstallRequest(w http.ResponseWriter, r *http.Request) error {
	var body *struct{}
	if err := decode(w, r, &body); err != nil {
		return err
	}
	if body == nil {
		return bad("请求必须为空 JSON 对象 {}，不能指定下载地址、版本或目标路径")
	}
	return nil
}
