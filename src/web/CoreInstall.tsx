import { ArrowClockwise, CheckCircle, CircleNotch, DownloadSimple, ShieldCheck, WarningCircle } from '@phosphor-icons/react';
import type { CoreInstallation } from '../shared/types';
import { useApp } from './state';
import { Badge, Button, Modal, Note, SectionHeading } from './ui';
import { coreBytes, coreDownloadPercent, coreStageLabel } from './CoreInstallModel';
import './CoreInstall.css';

function CoreProgress({ installation }: { installation: CoreInstallation }) {
  const percent = coreDownloadPercent(installation);
  return <div className="core-install-progress">
    <p role="status" aria-live="polite" aria-atomic="true"><CircleNotch size={17} className="spin" aria-hidden="true" /><strong>{coreStageLabel(installation)}</strong></p>
    {percent !== null && <progress max={100} value={percent} aria-label="安装包下载进度" aria-valuetext={percent + '%，' + coreBytes(installation.bytesDownloaded) + ' / ' + coreBytes(installation.totalBytes)} />}
    {installation.stage === 'downloading' && <span className="core-install-transfer">已下载 {coreBytes(installation.bytesDownloaded)}{installation.totalBytes > 0 ? ' / ' + coreBytes(installation.totalBytes) + ' · ' + percent + '%' : ' · 总大小未知'}</span>}
    <span className="field-hint">每 5 秒同步。关闭对话框或切换页面不会取消后台安装。</span>
  </div>;
}
function CoreSummary() {
  const { data } = useApp();
  const { coreInstallation: installation, runtime } = data!;
  if (installation.state === 'installing') return <CoreProgress installation={installation} />;
  if (runtime.available) return <Note tone="success">{installation.state === 'succeeded' ? '安装完成，版本校验通过。' : '已检测到可用的 Mihomo，无需重复安装。'}{runtime.running ? '当前代理状态以运行概况为准。' : '代理尚未启动；安装不会自动启动代理，请在运行概况中点击“重新应用配置”。'}</Note>;
  if (installation.state === 'failed') return <div className="core-install-feedback core-install-feedback--error" role="alert"><WarningCircle size={20} aria-hidden="true" /><div><strong>安装失败</strong><p>{installation.error || '安装未完成，请检查服务端状态后重试。'}</p>{!installation.canInstall && <p>{installation.reason || '当前不能重试；请先核实内核文件和运行状态。'}</p>}</div></div>;
  if (installation.state === 'succeeded') return <Note tone="warning">安装记录已完成，但当前内核尚不可用。请同步状态并核实运行错误，必要时修正文件后重新检测。安装不会自动启动代理；确认可用后仍需重新应用配置。</Note>;
  if (installation.installed) return <Note tone="warning">{installation.reason || '安装目标已有文件，但内核不可用。不会覆盖已有文件；请在服务主机手动修正路径或兼容版本后重新检测。'}</Note>;
  if (!installation.canInstall) return <Note tone="warning">{installation.reason || '当前平台或运行状态不支持在线安装，请手动配置兼容内核后重新检测。'}</Note>;
  return <Note>尚未检测到 Mihomo。可由管理服务在线安装固定版本，无需独立安装器；也可以手动放置内核文件后重新检测。安装不会自动启动代理。</Note>;
}
function CoreMetadata() {
  const { data } = useApp(); const installation = data!.coreInstallation;
  return <dl className="core-install-metadata">
    <div><dt>固定安装版本</dt><dd className="mono">Mihomo {installation.version}</dd></div>
    <div><dt>服务主机平台</dt><dd className="mono">{installation.os} / {installation.arch}</dd></div>
    <div className="core-install-path"><dt>安装目标（服务主机）</dt><dd className="mono">{installation.path || '服务端未提供安装路径'}</dd></div>
  </dl>;
}
function CoreRequestError() {
  const { coreError } = useApp();
  // Do not move focus from another dialog or from the global navigation when a
  // background response fails. The persistent live message is not toast-only.
  return coreError ? <div className="core-install-feedback core-install-feedback--error" role="alert"><WarningCircle size={19} aria-hidden="true" /><p>{coreError}</p></div> : null;
}
function CoreInstallActions({ dialog = false }: { dialog?: boolean }) {
  const { data, coreAction, startCoreInstall, inspectCore, closeCoreInstaller } = useApp();
  const { coreInstallation: installation, runtime } = data!;
  const installing = installation.state === 'installing';
  const disabled = installing || !!coreAction || runtime.applying;
  const canInstall = installation.canInstall && installation.supported && !installation.installed && !runtime.running;
  return <div className={dialog ? 'form-footer core-install-actions' : 'core-install-actions'}>
    {dialog && <Button onClick={closeCoreInstaller}>{installing ? '后台继续安装' : '稍后处理'}</Button>}
    <Button onClick={() => void inspectCore()} disabled={disabled} busy={coreAction === 'inspect'}><ArrowClockwise size={17} aria-hidden="true" />重新检测内核</Button>
    {canInstall && !installing && <Button variant="primary" onClick={() => void startCoreInstall()} disabled={disabled} busy={coreAction === 'install'}><DownloadSimple size={17} aria-hidden="true" />{installation.state === 'failed' ? '重试安装' : '在线安装 Mihomo'}</Button>}
    {installing && <span className="core-install-working"><CircleNotch size={16} className="spin" aria-hidden="true" />安装进行中</span>}
  </div>;
}
export function CoreInstallPanel() {
  const { openCoreInstaller } = useApp();
  return <section className="panel core-install-panel" aria-label="Mihomo 内核安装">
    <SectionHeading title="内核安装" description="安装与运行分开管理；不会覆盖已有文件或自动开启代理。" action={<Button small onClick={openCoreInstaller}>安装详情</Button>} />
    <CoreSummary /><CoreMetadata /><CoreRequestError /><CoreInstallActions />
  </section>;
}
export function CoreInstallDialog() {
  const { data, coreInstallOpen, closeCoreInstaller, authRequired } = useApp();
  if (!data || authRequired || !coreInstallOpen) return null;
  return <Modal title="安装 Mihomo 内核" description="管理服务可以保存配置；实际代理需要可用的 Mihomo。在线安装由服务主机后台执行，不需要独立安装器。" onClose={closeCoreInstaller}>
    <div className="dialog-body core-install-dialog"><CoreSummary /><CoreMetadata /><div className="core-install-assurance"><ShieldCheck size={19} aria-hidden="true" /><p>仅下载固定版本的官方发行包并校验 SHA256 与内核版本。已有目标文件不会被覆盖；不修改系统代理。</p></div><CoreRequestError /></div>
    <CoreInstallActions dialog />
  </Modal>;
}
export function CoreInstallBanner() {
  const { data, coreAction, coreError, openCoreInstaller } = useApp();
  if (!data) return null;
  const installation = data.coreInstallation;
  const installing = installation.state === 'installing';
  const succeeded = installation.state === 'succeeded' && data.runtime.available && !data.runtime.running;
  const failed = installation.state === 'failed' && !data.runtime.available;
  if (!installing && !coreAction && !succeeded && !failed && !coreError) return null;
  return <section className="core-install-banner" aria-label="内核安装状态">
    <div className="core-install-banner-heading" role="status" aria-live="polite" aria-atomic="true">{installing || coreAction ? <CircleNotch size={20} className="spin" aria-hidden="true" /> : succeeded ? <CheckCircle size={20} aria-hidden="true" /> : <WarningCircle size={20} aria-hidden="true" />}<strong>{coreAction === 'inspect' ? '正在重新检测内核' : coreAction === 'install' && !installing ? '正在提交安装请求' : coreError && !installing ? '内核安装操作未完成' : 'Mihomo · ' + coreStageLabel(installation)}</strong><Badge tone={installing ? 'teal' : succeeded ? 'success' : 'warning'}>{installation.version}</Badge></div>
    <p>{installing ? '后台安装会继续。可在任意页面打开详情查看进度。' : succeeded ? '安装完成，不会自动启动代理。请在运行概况中重新应用配置。' : coreAction ? '请稍候，以服务端返回的状态为准。' : coreError || installation.error || '安装失败，请打开详情核实后重试。'}</p>
    <Button small onClick={openCoreInstaller}>{installing ? '查看安装进度' : '查看安装详情'}</Button>
  </section>;
}
