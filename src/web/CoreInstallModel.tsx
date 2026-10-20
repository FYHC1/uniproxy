import type { CoreInstallation, RuntimeStatus } from '../shared/types';

export function isCoreInstallation(value: unknown): value is CoreInstallation {
  if (!value || typeof value !== 'object') return false;
  const item = value as Record<string, unknown>;
  return typeof item.state === 'string' && ['idle', 'installing', 'succeeded', 'failed'].includes(item.state)
    && ['stage', 'version', 'os', 'arch', 'path', 'reason', 'error', 'startedAt', 'finishedAt'].every(key => typeof item[key] === 'string')
    && ['installed', 'supported', 'canInstall'].every(key => typeof item[key] === 'boolean')
    && ['bytesDownloaded', 'totalBytes'].every(key => typeof item[key] === 'number' && Number.isFinite(item[key]) && (item[key] as number) >= 0);
}

export function shouldPromptForCore(installation: CoreInstallation, runtime: RuntimeStatus, prompted: ReadonlySet<string>): boolean {
  return !installation.installed && !runtime.available
    && installation.state !== 'installing' && !prompted.has(installation.path);
}

export function coreStageLabel(installation: CoreInstallation): string {
  if (installation.state === 'succeeded') return '安装完成';
  if (installation.state === 'failed') return '安装失败';
  if (installation.state !== 'installing') return installation.installed ? '已检测到文件' : '尚未安装';
  const stages: Record<string, string> = {
    metadata: '获取发布信息', downloading: '下载安装包', verifying: '校验 SHA256',
    extracting: '解压安装包', installing: '写入内核文件', inspecting: '检查内核版本', complete: '完成安装检查',
  };
  return stages[installation.stage] || '准备安装';
}

// Only the download transfer has a measurable percentage. Verification and
// version inspection must not look complete just because all bytes arrived.
export function coreDownloadPercent(installation: CoreInstallation): number | null {
  if (installation.state !== 'installing' || installation.stage !== 'downloading' || installation.totalBytes <= 0) return null;
  return Math.min(100, Math.max(0, Math.floor(installation.bytesDownloaded / installation.totalBytes * 100)));
}

export function coreBytes(bytes: number): string {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KiB';
  return (bytes / 1024 / 1024).toFixed(1) + ' MiB';
}
