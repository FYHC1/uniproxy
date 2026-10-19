import type { ImportIssue } from '../shared/types';

export const MAX_IMPORT_BYTES = 5 * 1024 * 1024;
export const MAX_IMPORT_ERROR_DETAILS = 128;

type FileHandle = {kind:string} | null;
type DropItem = Pick<DataTransferItem,'kind'|'getAsFile'> & {
  webkitGetAsEntry?: () => {isDirectory:boolean} | null;
  getAsFileSystemHandle?: () => Promise<FileHandle>;
};
type FileTransfer = Pick<DataTransfer,'types'|'files'> & {items:ArrayLike<DropItem>};
export interface LocalFileSelection {
  files: File[];
  itemCount?: number;
  directory?: boolean;
  unreadable?: boolean;
  handles?: Promise<FileHandle>[];
}
export function isFileTransfer(transfer: FileTransfer | null): boolean {
  return !!transfer && (Array.from(transfer.types).includes('Files') || transfer.files.length > 0 || Array.from(transfer.items).some(item => item.kind === 'file'));
}
/** Hover data is protected in browsers; reject what is knowable, validate again on drop. */
export function canDropLocalFile(transfer: FileTransfer): boolean {
  if (!isFileTransfer(transfer)) return false;
  const items = Array.from(transfer.items).filter(item => item.kind === 'file');
  if (items.length > 1 || transfer.files.length > 1) return false;
  try {
    if (items.some(item => item.webkitGetAsEntry?.()?.isDirectory)) return false;
    const file = transfer.files[0] || items[0]?.getAsFile();
    return !file || (!file.webkitRelativePath && file.size <= MAX_IMPORT_BYTES);
  } catch { return false; }
}
/** Snapshot Files and handle promises synchronously: the drag data store expires after drop. */
export function captureDroppedFiles(transfer: FileTransfer): LocalFileSelection {
  const items = Array.from(transfer.items).filter(item => item.kind === 'file');
  const selection: LocalFileSelection = {files:Array.from(transfer.files),itemCount:items.length,directory:false,handles:[]};
  const itemFiles: File[] = [];
  for (const item of items) {
    try {
      if (item.webkitGetAsEntry?.()?.isDirectory) selection.directory = true;
      const file = item.getAsFile();
      if (file) itemFiles.push(file);
      if (item.getAsFileSystemHandle) {
        // Attach rejection handling now, including for a selection rejected before awaiting.
        selection.handles!.push(item.getAsFileSystemHandle().catch(() => {selection.unreadable = true; return null;}));
      }
    } catch { selection.unreadable = true; }
  }
  if (!selection.files.length) selection.files = itemFiles;
  return selection;
}
export function validateImportContent(text: string, emptyMessage = '请输入需要导入的内容。'): void {
  if (!text.trim()) throw new Error(emptyMessage);
  if (new TextEncoder().encode(text).length > MAX_IMPORT_BYTES) throw new Error('导入内容超过 5 MiB，请拆分后重试。');
}
/** Browse and drop use the same validation and read path. No uploads happen here. */
export async function readLocalImport(selection: LocalFileSelection): Promise<{file:File;content:string}> {
  if (selection.directory || selection.files.some(file => !!file.webkitRelativePath)) throw new Error('不支持导入文件夹，请选择文件夹中的一个配置文件。');
  if (selection.files.length > 1 || (selection.itemCount || 0) > 1) throw new Error('一次只能导入一个文件，请分别导入。');
  const handles = await Promise.all(selection.handles || []);
  if (handles.some(handle => handle?.kind === 'directory')) throw new Error('不支持导入文件夹，请选择文件夹中的一个配置文件。');
  if (selection.unreadable) throw new Error('无法读取拖放项，请使用浏览文件选择一个配置文件。');
  const file = selection.files[0];
  if (!file) throw new Error('请拖入一个本地配置文件，或点击浏览文件。');
  if (file.size > MAX_IMPORT_BYTES) throw new Error('文件超过 5 MiB，请拆分后导入。');
  let content: string;
  try { content = await file.text(); }
  catch { throw new Error('无法读取文件，请重新选择，或通过文本方式粘贴内容。'); }
  validateImportContent(content,'文件内容为空，请选择有效的节点配置。');
  return {file,content};
}

export interface ImportDiagnostics {errors:ImportIssue[];totalErrors?:number;errorsTruncated?:boolean}
/** Preserve truthful totals for new servers and a conservative fallback for older ones. */
export function importErrorSummary(value: ImportDiagnostics) {
  const reported = typeof value.totalErrors === 'number' && Number.isSafeInteger(value.totalErrors) && value.totalErrors >= value.errors.length ? value.totalErrors : undefined;
  const total = reported ?? value.errors.length;
  const exact = !value.errorsTruncated || (reported !== undefined && reported > value.errors.length);
  const details = value.errors.slice(0,MAX_IMPORT_ERROR_DETAILS);
  const truncated = !!value.errorsTruncated || total > details.length;
  return {total,exact,details,truncated,countLabel:exact ? String(total) : total ? '至少 ' + total : '数量未知的'};
}
