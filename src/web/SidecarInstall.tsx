import { ArrowClockwise, CheckCircle, CircleNotch, DownloadSimple, WarningCircle } from '@phosphor-icons/react';
import { useState } from 'react';
import type { SidecarInfo } from '../shared/types';
import { api, errorMessage } from './api';
import { coreBytes, coreDownloadPercent, coreStageLabel } from './CoreInstallModel';
import { Badge, Button, Note } from './ui';

function SidecarInstallStatus({ sidecar, refresh }: { sidecar: SidecarInfo; refresh: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const installation = sidecar.installation;
  if (!installation) return null;
  const installing = installation.state === 'installing';
  const percent = coreDownloadPercent(installation);
  async function install() {
    setBusy(true);
    setError('');
    try {
      await api('/api/sidecars/install', 'POST', {});
      await refresh();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  return <div className="sidecar-install">
    <div className="sidecar-install-heading">
      <div><strong>Xray 可执行文件</strong><span>固定版本由管理服务在线安装，SHA256 与版本检测通过后才会启用。安装不会自动启动代理。</span></div>
      <div className="inline-controls">
        <Badge tone={installing ? 'teal' : installation.state === 'succeeded' ? 'success' : installation.state === 'failed' ? 'warning' : 'neutral'}>{installing ? coreStageLabel(installation) : installation.state === 'succeeded' ? '已安装' : installation.state === 'failed' ? '安装失败' : installation.installed ? '已检测到' : '未安装'}</Badge>
        {!installing && <Button small busy={busy || sidecar.status.running} disabled={busy || sidecar.status.running || !installation.canInstall} onClick={() => void install()}><DownloadSimple size={15} />{installation.state === 'failed' ? '重试安装' : '在线安装 Xray'}</Button>}
        <Button small busy={busy && installing} onClick={() => void refresh()}><ArrowClockwise size={15} />刷新</Button>
      </div>
    </div>
    <div className="sidecar-install-meta"><span>固定版本 <strong className="mono">Xray {installation.version}</strong></span><span>服务主机 <strong className="mono">{installation.os} / {installation.arch}</strong></span></div>
    <div className="sidecar-install-path"><span>安装目标</span><code>{installation.path || '服务端未提供安装路径'}</code></div>
    {installing && <div className="core-install-progress"><p role="status" aria-live="polite" aria-atomic="true"><CircleNotch size={17} className="spin" aria-hidden="true" /><strong>{coreStageLabel(installation)}</strong></p>{percent !== null && <progress max={100} value={percent} aria-label="Xray 安装包下载进度" aria-valuetext={percent + '%，' + coreBytes(installation.bytesDownloaded) + ' / ' + coreBytes(installation.totalBytes)} />}{installation.stage === 'downloading' && <span className="core-install-transfer">已下载 {coreBytes(installation.bytesDownloaded)}{installation.totalBytes > 0 ? ' / ' + coreBytes(installation.totalBytes) + ' · ' + percent + '%' : ' · 总大小未知'}</span>}<span className="field-hint">每 5 秒同步。关闭页面不会取消后台安装。</span></div>}
    {!installing && installation.state === 'failed' && <div className="core-install-feedback core-install-feedback--error" role="alert"><WarningCircle size={19} aria-hidden="true" /><div><strong>安装失败</strong><p>{installation.error || '安装未完成，请检查服务端状态后重试。'}</p>{!installation.canInstall && <p>{installation.reason || '当前不能重试；请先核实安装目录和文件状态。'}</p>}</div></div>}
    {!installing && installation.state === 'succeeded' && <Note tone="success"><CheckCircle size={14} /> Xray 已安装并通过版本检测。{sidecar.nodeCount > 0 && !sidecar.status.running ? '请在保存设置后重新应用配置，以启动 Xray sidecar。' : '当前代理状态以运行概况为准。'}</Note>}
    {!installing && installation.state !== 'succeeded' && installation.state !== 'failed' && !installation.canInstall && <Note tone="warning">{installation.reason || '当前平台或运行状态不支持在线安装，请手动配置固定版本 Xray 后重新检测。'}</Note>}
    {!installing && installation.state !== 'succeeded' && installation.state !== 'failed' && installation.canInstall && <Note>尚未检测到固定版本 Xray。可由管理服务在线安装，无需独立安装器；也可以手动放置可执行文件后重新检测。</Note>}
    {error && <Note tone="warning">安装请求未完成：{error}</Note>}
  </div>;
}

export function SidecarInstall({ sidecar, refresh }: { sidecar: SidecarInfo | null; refresh: () => Promise<void> }) {
  if (!sidecar || !sidecar.installation) return null;
  return <SidecarInstallStatus sidecar={sidecar} refresh={refresh} />;
}
