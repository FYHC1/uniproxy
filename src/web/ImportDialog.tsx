import { useEffect, useId, useRef, useState, type ChangeEvent, type DragEvent } from 'react';
import { ArrowRight, CheckCircle, FileText, LinkSimple, UploadSimple } from '@phosphor-icons/react';
import type { ImportRequest, ImportResult, ProxyNode } from '../shared/types';
import { useAction, useApp } from './state';
import { errorMessage } from './api';
import { Badge, Button, ErrorBox, Field, GroupPicker, Modal, Note, TableRegion } from './ui';
import { address } from './utils';
import { canDropLocalFile, captureDroppedFiles, importErrorSummary, isFileTransfer, readLocalImport, validateImportContent, type ImportDiagnostics, type LocalFileSelection } from './importUtils';

type Preview = ImportDiagnostics & {nodes:(Pick<ProxyNode,'name'|'protocol'|'server'|'port'|'udp'> & {dialerProxy?:string})[]};
export function ImportDialog({ onClose, initialMode = 'text' }: {onClose:()=>void; initialMode?:ImportRequest['mode']}) {
  const { data, request, mutate } = useApp();
  const [mode, setMode] = useState<ImportRequest['mode']>(initialMode);
  const [content, setContent] = useState('');
  const [filename, setFilename] = useState('');
  const [name, setName] = useState('');
  const [interval, setInterval] = useState(60);
  const [groupIds, setGroupIds] = useState<string[]>([]);
  const [groupNames, setGroupNames] = useState('');
  const [preview, setPreview] = useState<Preview | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [reading, setReading] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [dragState, setDragState] = useState<'valid'|'invalid'|null>(null);
  const action = useAction();
  const fileRef = useRef<HTMLInputElement>(null);
  const readEpoch = useRef(0);
  const previewEpoch = useRef(0);
  const importing = useRef(false);
  const mounted = useRef(true);
  const dragDepth = useRef(0);
  const fileHint = useId();
  useEffect(() => {
    mounted.current = true;
    // File drops outside the target must not navigate away from this dialog either.
    const resetDrag = () => {dragDepth.current = 0; setDragState(null);};
    const preventNavigation = (event:globalThis.DragEvent) => {
      if (!isFileTransfer(event.dataTransfer)) return;
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'none';
      resetDrag();
    };
    window.addEventListener('dragover',preventNavigation);
    window.addEventListener('drop',preventNavigation);
    window.addEventListener('dragend',resetDrag);
    window.addEventListener('blur',resetDrag);
    return () => {
      mounted.current = false;
      readEpoch.current++; previewEpoch.current++;
      window.removeEventListener('dragover',preventNavigation);
      window.removeEventListener('drop',preventNavigation);
      window.removeEventListener('dragend',resetDrag);
      window.removeEventListener('blur',resetDrag);
    };
  },[]);
  const payload = ():ImportRequest => ({mode, content, groupIds, groupNames:groupNames.split(/[,，\n]/).map(s => s.trim()).filter(Boolean), ...(name.trim() ? {name:name.trim()} : {}), ...(mode === 'subscriptions' ? {intervalMinutes:interval} : {})});
  function invalidatePreview() {
    previewEpoch.current++;
    setPreview(null); setPreviewing(false); action.setError('');
  }
  function updateContent(value:string) {
    readEpoch.current++; setReading(false);
    setContent(value); invalidatePreview();
  }
  function changeMode(value:ImportRequest['mode']) {
    if (importing.current || mode === value) return;
    setMode(value); updateContent(''); setFilename(''); dragDepth.current = 0; setDragState(null);
  }
  async function loadFile(selection:LocalFileSelection) {
    if (importing.current) return;
    const epoch = ++readEpoch.current;
    invalidatePreview(); setFilename(''); setContent(''); setReading(true);
    try {
      const next = await readLocalImport(selection);
      if (!mounted.current || epoch !== readEpoch.current) return;
      setContent(next.content); setFilename(next.file.name);
    } catch (error) {
      if (mounted.current && epoch === readEpoch.current) action.setError(errorMessage(error));
    } finally {
      if (mounted.current && epoch === readEpoch.current) setReading(false);
    }
  }
  function chooseFile(event:ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.currentTarget.files || []);
    // Reset synchronously, so a cancelled picker preserves state and the same file can be retried.
    event.currentTarget.value = '';
    if (files.length) void loadFile({files});
  }
  function dragOver(event:DragEvent<HTMLDivElement>) {
    if (!isFileTransfer(event.dataTransfer)) return;
    event.preventDefault(); event.stopPropagation();
    const valid = !importing.current && canDropLocalFile(event.dataTransfer);
    // Allow an invalid file drop to reach validation and invalidate any old preview.
    event.dataTransfer.dropEffect = importing.current ? 'none' : 'copy';
    setDragState(valid ? 'valid' : 'invalid');
  }
  function dropFile(event:DragEvent<HTMLDivElement>) {
    event.preventDefault(); event.stopPropagation();
    dragDepth.current = 0; setDragState(null);
    if (!importing.current) void loadFile(captureDroppedFiles(event.dataTransfer));
  }
  async function runPreview() {
    if (reading || importing.current) return;
    const epoch = ++previewEpoch.current;
    setPreview(null); setPreviewing(true); action.setError('');
    try {
      validateImportContent(content,mode === 'file' ? '请先选择本地配置文件。' : '请输入需要导入的内容。');
      const next = await request<Preview>('/api/import', 'POST', {...payload(),preview:true});
      if (mounted.current && epoch === previewEpoch.current) setPreview(next);
    } catch (error) {
      if (mounted.current && epoch === previewEpoch.current) action.setError(errorMessage(error));
    } finally {
      if (mounted.current && epoch === previewEpoch.current) setPreviewing(false);
    }
  }
  async function importNodes() {
    if (importing.current || reading || previewing) return;
    importing.current = true;
    await action.execute(async () => {
      if (!preview || !preview.nodes.length) throw new Error('请先完成预览，并确认至少有一个可导入节点。');
      const saved = await mutate<ImportResult>('/api/import','POST',payload(),'导入结果已保存');
      if (mounted.current) {setResult(saved); setContent(''); setPreview(null);}
    });
    importing.current = false;
  }
  const relayCount = preview?.nodes.filter(node => node.dialerProxy).length || 0;
  return <Modal title="导入节点" description="从分享链接、订阅或本地配置导入。先预览，确认后再写入。" onClose={onClose} busy={action.running} wide>
    {result ? <><div className="dialog-body import-result"><CheckCircle size={44} weight="duotone" className="text-teal" aria-hidden="true" /><h3>导入处理完成</h3><div className="result-grid">{[['新增',result.added],['更新',result.updated],['移除',result.removed],['跳过',result.skipped]].map(([label,count]) => <div key={label}><strong>{count}</strong><span>{label}</span></div>)}</div>{result.subscriptionIds.length > 0 && <p className="muted">已关联 {result.subscriptionIds.length} 个订阅源，可在订阅管理中查看。</p>}<ImportErrors {...result} subscriptions={mode === 'subscriptions'} /><Note>保存配置不代表代理已生效。请查看页面顶部的内核状态与待应用提示。</Note></div><div className="form-footer"><Button variant="primary" onClick={onClose}>完成</Button></div></> : <form onSubmit={event => { event.preventDefault(); void importNodes(); }}>
      <div className="dialog-body"><fieldset disabled={action.running} className="form-fields"><div className="segmented" role="group" aria-label="导入方式">{([{id:'text',label:'文本 / 分享链接',Icon:FileText},{id:'subscriptions',label:'订阅链接',Icon:LinkSimple},{id:'file',label:'本地文件',Icon:UploadSimple}] as const).map(item => <button type="button" key={item.id} aria-pressed={mode === item.id} className={mode === item.id ? 'active' : ''} onClick={() => changeMode(item.id)}><item.Icon size={18} aria-hidden="true" />{item.label}</button>)}</div>
      {mode === 'file' ? <div
        className={'file-drop' + (dragState ? ' file-drop--' + dragState : '')}
        role="group" aria-label="本地文件导入" aria-describedby={fileHint} aria-busy={reading}
        onDragEnter={event => {if (isFileTransfer(event.dataTransfer)) {dragDepth.current++; dragOver(event);}}}
        onDragOver={dragOver}
        onDragLeave={event => {event.stopPropagation(); dragDepth.current = Math.max(0,dragDepth.current - 1); if (!dragDepth.current) setDragState(null);}}
        onDrop={dropFile}
      ><UploadSimple size={28} weight="duotone" aria-hidden="true" /><strong>{filename || '拖放一个配置文件到此处'}</strong><p id={fileHint}>支持 YAML、JSON、文本 / CONF 和 Base64 订阅 · 单个文件，最大 5 MiB · 不支持文件夹</p><p className="file-drop-status" role="status" aria-live="polite" aria-atomic="true">{dragState === 'valid' ? '松开以读取文件' : dragState === 'invalid' ? '请选择单个不超过 5 MiB 的文件；不支持文件夹' : reading ? '正在读取文件，可重新选择或切换导入方式…' : filename ? '文件已读取，请预览后确认导入。' : '也可点击浏览文件，或切换到文本方式粘贴内容。'}</p><input ref={fileRef} type="file" accept=".txt,.yaml,.yml,.json,.conf" aria-label="选择节点配置文件" aria-describedby={fileHint} onChange={chooseFile} /><Button onClick={() => fileRef.current?.click()}>{filename ? '重新选择文件' : '浏览文件'}</Button></div> : <Field label={mode === 'text' ? '节点内容' : '订阅 URL'} required hint={mode === 'text' ? '支持 VLESS、VMess、Trojan、Shadowsocks、Hysteria、TUIC、SSH、SOCKS5、HTTP，以及 Clash / sing-box、Quantumult X、Surge / Surfboard、Loon 的静态节点配置。仅提取节点，不采用全局规则或脚本。' : '每行一个 HTTP(S) 地址。完整地址仅用于请求，列表中会隐藏凭据和查询参数。'}><textarea className="mono" rows={mode === 'text' ? 7 : 4} value={content} onChange={event => updateContent(event.target.value)} placeholder={mode === 'text' ? '粘贴分享链接、Base64 订阅或配置内容…' : '每行粘贴一个订阅地址…'} spellCheck={false} autoComplete="off" /></Field>}
      <p className="field-hint">Quantumult X、Surge / Surfboard、Loon 静态文本仅提取本地节点，不采用规则或脚本。未明确声明 UDP 时仅导入 TCP；需要 UDP 请在源中设置 udp-relay=true 或对应选项。</p>
      {mode === 'subscriptions' && <Note>预览会由管理服务直连下载，不使用浏览器或系统 HTTP 代理。检测到 Fake-IP DNS 时会仅将域名提交到受信任的公共加密 DNS 复核，再校验并连接公网地址；不会发送订阅路径或令牌，也不会放开内网访问。仍失败时可先在受信任客户端下载，再以本地文件导入。</Note>}
      {mode === 'subscriptions' && <div className="form-grid"><Field label="订阅名称" hint="多条链接由服务分别建立订阅。"><input value={name} onChange={event => {setName(event.target.value); invalidatePreview();}} placeholder="可选，用于识别来源" maxLength={100} /></Field><Field label="刷新间隔（分钟）"><input type="number" min={5} max={10080} required value={interval} onChange={event => {setInterval(Number(event.target.value)); invalidatePreview();}} /></Field></div>}
      <div className="form-grid"><GroupPicker groups={data!.groups} value={groupIds} onChange={value => {setGroupIds(value); invalidatePreview();}} /><Field label="同时新建分组" hint="使用逗号分隔；仅在确认导入时创建。"><input value={groupNames} onChange={event => {setGroupNames(event.target.value); invalidatePreview();}} placeholder="例如：工作节点, 备用线路" maxLength={1000} /></Field></div>
      </fieldset><ErrorBox message={action.error} />
      {preview && <section className="preview-section"><div className="section-heading"><h3>导入预览 <Badge tone="teal">{preview.nodes.length} 个节点</Badge></h3><span className="muted">尚未保存</span></div><ImportErrors {...preview} subscriptions={mode === 'subscriptions'} />{relayCount > 0 && <Note>包含 {relayCount} 个中转节点：将通过同一配置中的关联节点连接，不会回退为直连；关联节点缺失或禁用时线路不可用。</Note>}{preview.nodes.length ? <TableRegion label="导入预览节点"><table className="data-table compact-table"><thead><tr><th scope="col">名称</th><th scope="col">协议</th><th scope="col">服务器</th><th scope="col">UDP</th><th scope="col">中转路由</th></tr></thead><tbody>{preview.nodes.slice(0,100).map((node,index) => <tr key={index}><td>{node.name}</td><td><Badge>{node.protocol.toUpperCase()}</Badge></td><td className="mono">{address(node.server,node.port)}</td><td>{node.udp ? '支持' : '不支持'}</td><td className="relay-preview">{node.dialerProxy ? '经由：' + node.dialerProxy : '无中转'}</td></tr>)}</tbody></table></TableRegion> : <Note tone="warning">{mode === 'subscriptions' ? '尚未获取到有效节点。请先按上方的下载或配置错误排查；零节点不等于订阅格式一定不支持。' : '未识别出可导入节点，请检查内容与解析错误。'}</Note>}{preview.nodes.length > 100 && <p className="field-hint">预览显示前 100 个节点；确认后导入全部 {preview.nodes.length} 个节点。</p>}</section>}
      <p className="privacy-note">节点凭据不会展示在预览中，也不会写入浏览器本地存储。</p></div>
      <div className="form-footer"><Button onClick={onClose} disabled={action.running}>取消</Button><Button busy={previewing} disabled={!content.trim() || reading || action.running} onClick={() => void runPreview()}>预览解析</Button><Button type="submit" variant="primary" busy={action.running} disabled={!preview?.nodes.length || reading || previewing || action.running}>确认导入 <ArrowRight size={16} aria-hidden="true" /></Button></div>
    </form>}
  </Modal>;
}
function ImportErrors(diagnostics:ImportDiagnostics & {subscriptions?:boolean}) {
  const {total,details,truncated,exact,countLabel} = importErrorSummary(diagnostics);
  if (!total && !truncated) return null;
  return <div className="import-errors"><strong>有 {countLabel} 项{diagnostics.subscriptions ? '获取或解析失败' : '未能解析'}</strong>{details.length > 0 && <ul>{details.map((error,index) => <li key={index}>{error.line > 0 ? diagnostics.subscriptions ? '第 ' + error.line + ' 个订阅：' : '第 ' + error.line + ' 行：' : ''}{error.message}</li>)}</ul>}{truncated && <p>错误详情已截断，仅显示 {details.length} 项{exact ? '（共 ' + total + ' 项）' : '；服务未提供完整错误总数'}。</p>}</div>;
}
