import { Component, useEffect, useState, type ErrorInfo, type ReactNode } from 'react';
import { ArrowClockwise, ArrowsSplit, CheckCircle, CircleNotch, CirclesThreePlus, FolderSimple, GearSix, LinkSimple, LockKey, Network, PlugsConnected, ShieldCheck, WarningCircle, X } from '@phosphor-icons/react';
import { useAction, useApp } from './state';
import { Badge, Button, ErrorBox, Field, IconButton } from './ui';
import { runtimeLabel, runtimeTone } from './utils';
import { NodesPage } from './NodesPage';
import { GroupsPage } from './GroupsPage';
import { PoolsPage } from './PoolsPage';
import { ListenersPage } from './ListenersPage';
import { SubscriptionsPage } from './SubscriptionsPage';
import { SettingsPage } from './SettingsPage';
import { CoreInstallBanner, CoreInstallDialog } from './CoreInstall';

type Page = 'nodes'|'groups'|'pools'|'listeners'|'subscriptions'|'settings';
const navigation = [
  {id:'nodes',label:'节点管理',en:'Node registry',Icon:Network},
  {id:'groups',label:'分组管理',en:'Organization',Icon:FolderSimple},
  {id:'pools',label:'代理池',en:'Proxy pools',Icon:ArrowsSplit},
  {id:'listeners',label:'监听端口',en:'Listeners',Icon:PlugsConnected},
  {id:'subscriptions',label:'订阅管理',en:'Subscriptions',Icon:LinkSimple},
  {id:'settings',label:'运行与设置',en:'Operations',Icon:GearSix}
] as const;
function currentPage():Page {const key = window.location.hash.replace('#/','');return navigation.some(item => item.id === key) ? key as Page : 'nodes';}
export function App() {
  const {data,loading,authRequired,connectionError,refreshing,refresh,updatedAt,notices,dismiss,forgetToken,hasToken,runtimeAction,openCoreInstaller,coreAction} = useApp();
  const [page,setPage] = useState<Page>(currentPage); const action = useAction();
  useEffect(() => {const change = () => {setPage(currentPage());window.requestAnimationFrame(() => document.getElementById('page-title')?.focus({preventScroll:true}));};window.addEventListener('hashchange',change);return () => window.removeEventListener('hashchange',change);},[]);
  useEffect(() => {document.title = (navigation.find(item => item.id === page)?.label || '管理控制台') + ' · UniProxy';},[page]);
  const active = navigation.find(item => item.id === page)!;
  const activeJobs = data?.jobs.filter(job => job.status === 'queued' || job.status === 'running').length || 0;
  if (authRequired) return <AuthScreen />;
  return <><a className="skip-link" href="#main-content" onClick={event => {event.preventDefault();document.getElementById('main-content')?.focus();}}>跳到主要内容</a><div className="app-shell"><aside className="sidebar" aria-label="主导航"><a className="brand" href="#/nodes" aria-label="UniProxy 节点管理"><span className="brand-mark"><CirclesThreePlus size={26} weight="fill" /></span><span>Uni<span className="brand-light">Proxy</span><small>代理管理工作台</small></span></a><div className="sidebar-workspace"><span className="workspace-symbol">U</span><div><strong>默认工作空间</strong><span>SELF-HOSTED</span></div><span className="workspace-indicator" /></div><div className="nav-label">工作空间</div><nav><ul>{navigation.map((item,index) => <li key={item.id} className={index === 5 ? 'nav-system' : ''}><a href={'#/' + item.id} aria-current={page === item.id ? 'page' : undefined} className={page === item.id ? 'nav-link active' : 'nav-link'} title={item.label} aria-label={item.label}><item.Icon size={20} weight={page === item.id ? 'fill' : 'regular'} /><span>{item.label}</span>{data && item.id === 'nodes' && <small>{data.nodes.length}</small>}{item.id === 'settings' && activeJobs > 0 && <small className="nav-job-count">{activeJobs}</small>}</a></li>)}</ul></nav><div className="sidebar-bottom"><div className="sidebar-core"><div><span className={'sidebar-status-dot' + (data?.runtime.running && !data.runtime.dirty ? ' online' : '')} /><strong>{data ? runtimeLabel(data.runtime) : '等待服务响应'}</strong></div><p>{data?.runtime.available ? data.runtime.version || 'Mihomo 核心' : '真实状态 · 不模拟运行'}</p><a href="#/settings">查看运行详情 <span aria-hidden="true">↗</span></a></div><div className="sidebar-footer"><span className="tiny-logo">U</span><span>UniProxy Console</span><span className="footer-dot" /></div></div></aside>
    <div className="main-shell"><header className="topbar"><div className="breadcrumbs"><span>工作空间</span><span className="breadcrumb-slash">/</span><strong>{active.label}</strong></div><div className="topbar-actions">{activeJobs > 0 && <a href="#/settings" className="job-status"><CircleNotch size={15} className="spin" />{activeJobs} 项任务</a>}{data && <Badge dot tone={runtimeTone(data.runtime)}>{runtimeLabel(data.runtime)}</Badge>}<span className="topbar-divider" /><IconButton label={refreshing ? '正在同步数据' : '立即同步数据'} disabled={refreshing} onClick={() => void refresh()}><ArrowClockwise size={18} className={refreshing ? 'spin' : ''} /></IconButton><IconButton label={hasToken ? '清除令牌并锁定控制台' : '输入管理令牌'} onClick={forgetToken}><LockKey size={18} /></IconButton><span className="operator-avatar" aria-label="当前管理会话">U</span></div></header>
      <main id="main-content" className="main-content" tabIndex={-1}>
      {connectionError && <div className="global-alert global-alert--error" role="alert"><WarningCircle size={22} /><div><strong>{data ? '与管理服务的连接已中断' : '无法加载工作空间'}</strong><p>{connectionError}{data ? ' 当前显示最近一次成功同步的数据。' : ''}</p></div><Button small busy={refreshing} onClick={() => void refresh()}>重试连接</Button></div>}
      {data && (!data.runtime.available || data.runtime.dirty || !!data.runtime.lastError) && <div className="global-alert global-alert--warning" role="status"><WarningCircle size={22} /><div><strong>{!data.runtime.available ? '内核尚不可用，配置仅保存到管理服务' : data.runtime.dirty ? '存在尚未应用的配置更改' : '内核报告了运行错误'}</strong><p>{data.runtime.lastError || (!data.runtime.available ? '节点和监听不会自动开始代理。请安装或配置 Mihomo，再应用当前配置。' : '期望配置与运行配置可能不同。解决内核错误后重新应用，勿将保存成功视为代理已生效。')}</p></div>{!data.runtime.available ? <Button small onClick={openCoreInstaller}>{data.coreInstallation.state === 'installing' ? '查看安装进度' : '管理内核安装'}</Button> : <Button small busy={action.running} disabled={data.runtime.applying || data.coreInstallation.state === 'installing' || !!coreAction} onClick={() => void action.execute(() => runtimeAction('apply'))}>重新应用</Button>}</div>}
      <CoreInstallBanner />
      {data?.runtime.applying && <div className="applying-banner" role="status"><CircleNotch size={16} className="spin" />正在验证并应用配置，请以完成后的运行状态为准。</div>}
      <ErrorBox message={action.error} />
      {!data ? loading ? <LoadingWorkspace /> : <div className="connection-empty"><div className="connection-symbol"><Network size={42} weight="duotone" /></div><h1 id="page-title" tabIndex={-1}>等待连接 UniProxy</h1><p>连接成功后展示真实节点、订阅与内核状态。<br />请先启动管理服务，或确认网络与管理令牌。</p><Button variant="primary" busy={refreshing} onClick={() => void refresh()}><ArrowClockwise size={17} />重新连接</Button></div> : <PageBoundary key={page}>{page === 'nodes' ? <NodesPage /> : page === 'groups' ? <GroupsPage /> : page === 'pools' ? <PoolsPage /> : page === 'listeners' ? <ListenersPage /> : page === 'subscriptions' ? <SubscriptionsPage /> : <SettingsPage />}</PageBoundary>}
      <footer className="main-footer"><span><ShieldCheck size={14} />仅展示真实配置与检测结果</span><span>{updatedAt ? '最近同步 ' + updatedAt.toLocaleTimeString('zh-CN',{hour12:false}) : '尚未同步'} · UniProxy</span></footer>
      </main></div></div><CoreInstallDialog /><div className="toast-region" aria-label="操作通知" aria-live="polite" aria-atomic="false">{notices.map(notice => <div className={'toast toast--' + notice.tone} key={notice.id} role={notice.tone === 'error' ? 'alert' : 'status'}>{notice.tone === 'success' ? <CheckCircle size={21} /> : <WarningCircle size={21} />}<p>{notice.text}</p><IconButton label="关闭通知" onClick={() => dismiss(notice.id)}><X size={16} /></IconButton></div>)}</div></>;
}
function AuthScreen() {
  const {authenticate,authError,refreshing,connectionError} = useApp(); const action = useAction(); const [token,setToken] = useState('');
  return <div className="auth-screen"><section className="auth-card"><div className="auth-brand"><span className="brand-mark"><CirclesThreePlus size={27} weight="fill" /></span><strong>UniProxy</strong></div><div className="eyebrow">SECURE WORKSPACE</div><h1>验证管理访问</h1><p>此管理服务需要 Bearer 令牌。输入服务端配置的管理令牌以继续。</p><form onSubmit={event => {event.preventDefault();void action.execute(async () => {if (!token.trim()) throw new Error('请输入管理令牌。');await authenticate(token);});}}><Field label="管理令牌" required hint="只保存在当前标签页会话中，关闭会话后失效。"><input type="password" value={token} onChange={event => setToken(event.target.value)} autoFocus autoComplete="off" required placeholder="输入管理令牌" spellCheck={false} /></Field><ErrorBox message={action.error || authError || connectionError} /><Button variant="primary" type="submit" busy={action.running || refreshing}><LockKey size={17} />验证并进入</Button><Button disabled={action.running || refreshing} onClick={() => void action.execute(() => authenticate(''))}>尝试本机免令牌访问</Button></form><div className="auth-note"><ShieldCheck size={19} /><span>令牌不会通过 URL 传递。<br />公开部署请使用 HTTPS 加密连接。</span></div></section><span className="auth-footer">UNIPROXY · SELF-HOSTED PROXY MANAGEMENT</span></div>;
}
function LoadingWorkspace() {return <div className="loading-workspace" role="status" aria-label="正在加载真实工作空间数据"><div className="skeleton skeleton-heading" /><div className="skeleton skeleton-description" /><div className="skeleton-stats">{[0,1,2,3].map(index => <div className="skeleton" key={index} />)}</div><div className="loading-panel"><CircleNotch size={25} className="spin" /><span>正在读取节点与运行状态…</span></div></div>;}
class PageBoundary extends Component<{children:ReactNode},{error:boolean}> {
  state = {error:false};
  static getDerivedStateFromError() {return {error:true};}
  componentDidCatch(_error:Error,_info:ErrorInfo) { /* Do not log potentially sensitive API content. */ }
  render() {if (this.state.error) return <div className="connection-empty"><WarningCircle size={36} /><h1 id="page-title">页面无法正常显示</h1><p>请重新加载管理数据。若问题持续，请核实前后端版本与 API 响应。</p><Button onClick={() => window.location.reload()}>重新加载页面</Button></div>;return this.props.children;}
}
