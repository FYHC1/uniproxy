import { cloneElement, isValidElement, useEffect, useId, useRef, useState, type ButtonHTMLAttributes, type ReactElement, type ReactNode } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { CheckCircle, CircleNotch, MagnifyingGlass, WarningCircle, X, Tray } from '@phosphor-icons/react';
import { useAction } from './state';
import type { Group } from '../shared/types';
import { selectScope } from './selection';

export function Button({ children, variant = 'secondary', small = false, busy = false, className = '', disabled, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary'|'secondary'|'ghost'|'danger'; small?: boolean; busy?: boolean }) {
  return <button type="button" {...props} disabled={disabled || busy} aria-busy={busy || undefined} className={'button button--' + variant + (small ? ' button--small' : '') + ' ' + className}>{busy ? <CircleNotch size={17} className="spin" aria-hidden="true" /> : null}{children}</button>;
}
export function IconButton({ label, children, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & {label:string}) { return <button type="button" {...props} aria-label={label} title={props.title || label} className={'icon-button ' + (props.className || '')}>{children}</button>; }
export function Badge({ children, tone = 'neutral', dot = false }: {children:ReactNode; tone?: 'neutral'|'success'|'warning'|'danger'|'teal'|'blue'; dot?:boolean}) { return <span className={'badge badge--' + tone}>{dot && <span className="status-dot" aria-hidden="true" />}{children}</span>; }
export function Field({ label, hint, children, required = false }: {label:string; hint?:ReactNode; children:ReactNode; required?:boolean}) {
  const id = useId();
  const control = isValidElement(children) ? cloneElement(children as ReactElement<{id?:string;'aria-describedby'?:string}>, {id,'aria-describedby':hint ? id + '-hint' : undefined}) : children;
  return <div className="field"><label htmlFor={id} className="field-label">{label}{required && <span className="required" aria-hidden="true" />}</label>{control}{hint && <span id={id + '-hint'} className="field-hint">{hint}</span>}</div>;
}
export function Toggle({ checked, onChange, label, description, disabled }: {checked:boolean; onChange:(checked:boolean)=>void; label:string; description?:string; disabled?:boolean}) {
  const id = useId();
  return <div className="toggle-field"><div><label htmlFor={id} className="field-label">{label}</label>{description && <p id={id + '-hint'} className="field-hint">{description}</p>}</div><button id={id} className="switch" type="button" role="switch" aria-label={label} aria-checked={checked} aria-describedby={description ? id + '-hint' : undefined} disabled={disabled} onClick={() => onChange(!checked)}><span /></button></div>;
}
export function Check({ checked, onChange, label, indeterminate = false, disabled = false }: {checked:boolean; onChange:(checked:boolean)=>void; label:string; indeterminate?:boolean; disabled?:boolean}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { if (ref.current) ref.current.indeterminate = indeterminate; }, [indeterminate]);
  return <input ref={ref} className="checkbox" type="checkbox" checked={checked} disabled={disabled} aria-label={label} onChange={event => onChange(event.target.checked)} />;
}
export function ErrorBox({ message }: {message:string}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { if (message) ref.current?.focus(); }, [message]);
  return message ? <div className="inline-alert inline-alert--error" role="alert" tabIndex={-1} ref={ref}><WarningCircle size={19} aria-hidden="true" /><span>{message}</span></div> : null;
}
export function Note({ children, tone = 'info' }: {children:ReactNode; tone?:'info'|'warning'|'success'}) { return <div className={'note note--' + tone}>{tone === 'success' ? <CheckCircle size={18} aria-hidden="true" /> : <WarningCircle size={18} aria-hidden="true" />}<div>{children}</div></div>; }
export function Modal({ title, description, children, onClose, busy = false, wide = false }: {title:string; description:string; children:ReactNode; onClose:()=>void; busy?:boolean; wide?:boolean}) {
  const trigger = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null);
  return <Dialog.Root open onOpenChange={open => { if (!open && !busy) onClose(); }}><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className={'dialog-content' + (wide ? ' dialog-content--wide' : '')} onEscapeKeyDown={event => { if (busy) event.preventDefault(); }} onPointerDownOutside={event => event.preventDefault()} onCloseAutoFocus={event => { event.preventDefault(); if (trigger.current?.isConnected) trigger.current.focus(); else document.getElementById('page-title')?.focus(); }}><div className="dialog-header"><div><Dialog.Title className="dialog-title">{title}</Dialog.Title><Dialog.Description className="dialog-description">{description}</Dialog.Description></div><Dialog.Close asChild><IconButton label="关闭对话框" disabled={busy}><X size={20} /></IconButton></Dialog.Close></div>{children}</Dialog.Content></Dialog.Portal></Dialog.Root>;
}
export function FormFooter({ onClose, busy, label = '保存更改', children }: {onClose:()=>void; busy?:boolean; label?:string; children?:ReactNode}) { return <div className="form-footer"><span className="form-footer-note">{children || '保存后自动应用配置'}</span><Button disabled={busy} onClick={onClose}>取消</Button><Button variant="primary" type="submit" busy={busy}>{label}</Button></div>; }
export interface Confirmation { title:string; description:string; detail?:ReactNode; label?:string; danger?:boolean; action:()=>Promise<void> }
export function ConfirmDialog({ confirmation, onClose }: {confirmation:Confirmation; onClose:()=>void}) {
  const action = useAction();
  return <Modal title={confirmation.title} description={confirmation.description} onClose={onClose} busy={action.running}><form onSubmit={event => { event.preventDefault(); void action.execute(async () => { await confirmation.action(); onClose(); }); }}><div className="dialog-body">{confirmation.detail && <Note tone={confirmation.danger ? 'warning' : 'info'}>{confirmation.detail}</Note>}<ErrorBox message={action.error} /></div><div className="form-footer"><Button autoFocus onClick={onClose} disabled={action.running}>取消</Button><Button type="submit" variant={confirmation.danger ? 'danger' : 'primary'} busy={action.running}>{confirmation.label || '确认操作'}</Button></div></form></Modal>;
}
export function EmptyState({ title, description, action, compact = false }: {title:string; description:string; action?:ReactNode; compact?:boolean}) { return <div className={'empty-state' + (compact ? ' empty-state--compact' : '')}><div className="empty-symbol"><Tray size={28} weight="duotone" aria-hidden="true" /></div><h3>{title}</h3><p>{description}</p>{action}</div>; }
export function PageHeader({ eyebrow = 'WORKSPACE', title, description, actions }: {eyebrow?:string; title:string; description:string; actions?:ReactNode}) { return <div className="page-heading"><div><div className="eyebrow">{eyebrow}</div><h1 id="page-title" tabIndex={-1}>{title}</h1><p>{description}</p></div>{actions && <div className="page-actions">{actions}</div>}</div>; }
export function SectionHeading({ title, description, action }: {title:string; description?:string; action?:ReactNode}) { return <div className="section-heading"><div><h2>{title}</h2>{description && <p>{description}</p>}</div>{action}</div>; }
export function SearchInput({ value, onChange, placeholder = '搜索名称或地址…', label = '搜索' }: {value:string; onChange:(value:string)=>void; placeholder?:string; label?:string}) { return <div className="search-field"><MagnifyingGlass size={18} aria-hidden="true" /><input type="search" aria-label={label} placeholder={placeholder} value={value} onChange={event => onChange(event.target.value)} /></div>; }
export function GroupTags({ groups, ids }: {groups:Group[]; ids:string[]}) {
  return ids.length ? <div className="tag-list">{ids.map(id => { const group = groups.find(g => g.id === id); return <span className="group-tag" key={id}><span className="group-dot" style={{backgroundColor:safeColor(group?.color)}} />{group?.name || '已删除分组'}</span>; })}</div> : <span className="muted">未分组</span>;
}
export function safeColor(color?:string) { return color && /^#[0-9a-f]{6}$/i.test(color) ? color : '#718096'; }
export interface ChoiceOption {id:string;name:string;detail?:string;disabled?:boolean}
export function ChoiceList({ label, options, value, onChange, empty = '暂无可选项目', searchable = false, bulk = false }: {label:string; options:ChoiceOption[]; value:string[]; onChange:(ids:string[])=>void; empty?:string; searchable?:boolean; bulk?:boolean}) {
  const [query, setQuery] = useState('');
  const [visibleCount, setVisibleCount] = useState(100);
  const filtered = options.filter(o => (o.name + ' ' + (o.detail || '')).toLowerCase().includes(query.trim().toLowerCase()));
  const selected = new Set(value);
  const selectable = filtered.filter(option => !option.disabled).map(option => option.id);
  const allSelected = selectable.length > 0 && selectable.every(id => selected.has(id));
  const someSelected = selectable.some(id => selected.has(id));
  return <fieldset className={'choice-field' + (bulk ? ' choice-field--bulk' : '')}>
    <legend>{label}<span className="selection-count">{value.length} 已选</span></legend>
    {searchable && <SearchInput value={query} onChange={next => {setQuery(next);setVisibleCount(100);}} label={'搜索' + label} placeholder="输入名称或地址筛选…" />}
    {bulk && <div className="choice-toolbar"><label className="choice-select-all"><Check checked={allSelected} indeterminate={!allSelected && someSelected} disabled={!selectable.length} label={'全选' + label + '当前结果'} onChange={checked => onChange(selectScope(value,selectable,checked))} /><span>全选当前结果 <strong>{selectable.length}</strong></span></label><span className="field-hint">覆盖全部搜索结果，不限当前显示项</span></div>}
    <div className="choice-list">{filtered.length ? filtered.slice(0,visibleCount).map(option => <label key={option.id} className={'choice-item' + (option.disabled ? ' choice-item--disabled' : '')}><Check checked={selected.has(option.id)} disabled={option.disabled} label={option.name} onChange={checked => onChange(selectScope(value,[option.id],checked))} /><span>{option.name}{option.detail && <small>{option.detail}</small>}</span></label>) : <p className="choice-empty">{query.trim() && options.length ? '没有匹配当前搜索的项目。' : empty}</p>}</div>
    {filtered.length > visibleCount && <Button small onClick={() => setVisibleCount(count => count + 100)}>再显示 100 项（共 {filtered.length} 项）</Button>}
    {value.some(id => !options.some(option => option.id === id)) && <p className="field-hint">部分已选项目不在当前可选范围。<button type="button" className="text-button" onClick={() => onChange(value.filter(id => options.some(o => o.id === id)))}>清理范围外选择</button></p>}
  </fieldset>;
}
export function GroupPicker({ groups, value, onChange, label = '所属分组', bulk = false }: {groups:Group[]; value:string[]; onChange:(ids:string[])=>void; label?:string; bulk?:boolean}) { return <ChoiceList label={label} options={groups} value={value} onChange={onChange} bulk={bulk} empty="尚无分组，可在分组管理中创建。" />; }
export function TableRegion({ children, label }: {children:ReactNode; label:string}) { return <div className="table-region" role="region" aria-label={label} tabIndex={0}>{children}</div>; }
export function Pagination({ total, page, pageSize, onPage }: {total:number; page:number; pageSize:number; onPage:(page:number)=>void}) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  return <div className="table-footer"><span>{total ? ((page - 1) * pageSize + 1) + '–' + Math.min(page * pageSize, total) + ' 条，共 ' + total + ' 条' : '共 0 条'}</span><div className="pagination"><Button small disabled={page <= 1} onClick={() => onPage(page - 1)}>上一页</Button><span>{page} / {pages}</span><Button small disabled={page >= pages} onClick={() => onPage(page + 1)}>下一页</Button></div></div>;
}
