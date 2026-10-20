package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/safehttp"
	"github.com/uniproxy/uniproxy/internal/store"
)

type Core interface {
	Inspect(context.Context) model.RuntimeStatus
	Status() model.RuntimeStatus
	Validate(context.Context, model.State) error
	Apply(context.Context, model.State) error
	Stop(context.Context) error
	Check(context.Context, string, string, time.Duration) (model.CheckResult, error)
	Speed(context.Context, string, string, time.Duration, int64) (model.CheckResult, error)
}
type Options struct {
	Token                     string
	Host                      string
	AllowedOrigins            []string
	AllowPrivateSubscriptions bool
	AdminPort                 int
	// Version is reported by /api/health so a running instance can be matched to
	// the release it came from. Empty means "development build".
	Version string
}
type App struct {
	mu              sync.Mutex
	store           *store.Store
	core            Core
	opts            Options
	client          *http.Client
	jobMu           sync.Mutex
	jobs            map[string]*model.Job
	cancels         map[string]context.CancelFunc
	background      context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	refreshing      map[string]bool
	lastHealth      time.Time
	applyError      string
	manuallyStopped bool
}
type apiError struct {
	Status        int
	Message, Code string
}

func (e *apiError) Error() string { return e.Message }
func bad(message string) error    { return &apiError{400, message, "validation"} }
func missing() error              { return &apiError{404, "记录不存在", "not_found"} }
func ID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func New(s *store.Store, c Core, opts Options) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{store: s, core: c, opts: opts, client: safehttp.NewClient(opts.AllowPrivateSubscriptions), jobs: map[string]*model.Job{}, cancels: map[string]context.CancelFunc{}, background: ctx, cancel: cancel, refreshing: map[string]bool{}}
}
func (a *App) Start() {
	a.mu.Lock()
	a.core.Inspect(a.background)
	a.applyLocked(a.store.Snapshot())
	a.mu.Unlock()
	a.wg.Add(1)
	go a.scheduler()
}
func (a *App) Close() {
	a.cancel()
	a.jobMu.Lock()
	for _, c := range a.cancels {
		c()
	}
	a.jobMu.Unlock()
	a.wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.core.Stop(ctx)
}
func (a *App) runtime() model.RuntimeStatus {
	r := a.core.Status()
	r.Dirty = r.Dirty || r.AppliedRevision != a.store.Snapshot().Revision || !r.Running || a.applyError != ""
	if r.Listeners == nil {
		r.Listeners = []model.ListenerRuntime{}
	}
	if a.applyError != "" {
		r.LastError = a.applyError
	}
	return r
}
func (a *App) Runtime() model.RuntimeStatus { a.mu.Lock(); defer a.mu.Unlock(); return a.runtime() }
func (a *App) applyLocked(s model.State) {
	if a.manuallyStopped {
		a.applyError = "内核已手动停止；更改仅保存，点击重新应用后恢复代理"
		return
	}
	ctx, cancel := context.WithTimeout(a.background, 45*time.Second)
	defer cancel()
	if e := a.core.Apply(ctx, s); e != nil {
		a.applyError = "配置已保存但尚未生效：" + e.Error()
		a.store.Activity("warning", a.applyError)
	} else {
		a.applyError = ""
	}
}
func (a *App) mutate(ctx context.Context, label string, fn func(*model.State) (any, error)) (any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.store.Snapshot()
	data, e := fn(&s)
	if e != nil {
		return nil, e
	}
	store.Normalize(&s)
	if e = ValidateState(s, a.opts.AdminPort); e != nil {
		return nil, e
	}
	s.Revision++
	validationCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if e = a.core.Validate(validationCtx, s); e != nil && a.core.Status().Available {
		return nil, bad("代理内核配置校验失败：" + e.Error())
	}
	if e = a.store.Save(s); e != nil {
		return nil, fmt.Errorf("保存配置失败: %w", e)
	}
	a.applyLocked(s)
	a.store.Activity("info", label)
	return map[string]any{"data": data, "runtime": a.runtime()}, nil
}
func (a *App) bootstrap() any {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.store.Snapshot()
	annotateRelays(&s)
	for i := range s.Nodes {
		s.Nodes[i].Config = nil
		s.Nodes[i].Fingerprint = ""
		s.Nodes[i].DialerIdentity = ""
		s.Nodes[i].SourceGroups = nil
		s.Nodes[i].ManualGroupIDs = nil
	}
	for i := range s.Listeners {
		s.Listeners[i] = publicListener(s.Listeners[i])
	}
	for i := range s.Subscriptions {
		s.Subscriptions[i].URL = safehttp.RedactURL(s.Subscriptions[i].URL)
	}
	return struct {
		model.State
		Runtime    model.RuntimeStatus `json:"runtime"`
		Activities []model.Activity    `json:"activities"`
		Jobs       []model.Job         `json:"jobs"`
	}{s, a.runtime(), a.store.Activities(), a.jobList()}
}
func publicListener(l model.Listener) model.Listener {
	l.Users = append([]model.Credential{}, l.Users...)
	for i := range l.Users {
		l.Users[i].HasPassword = l.Users[i].Password != ""
		l.Users[i].Password = ""
	}
	return l
}
func nodeIndex(s *model.State, id string) int {
	for i, n := range s.Nodes {
		if n.ID == id {
			return i
		}
	}
	return -1
}
func poolIndex(s *model.State, id string) int {
	for i, p := range s.Pools {
		if p.ID == id {
			return i
		}
	}
	return -1
}
func contains(xs []string, s string) bool {
	for _, v := range xs {
		if v == s {
			return true
		}
	}
	return false
}
func union(xs, ys []string) []string {
	out := append([]string{}, xs...)
	for _, v := range ys {
		if !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
func without(xs []string, ids []string) []string {
	out := []string{}
	for _, v := range xs {
		if !contains(ids, v) {
			out = append(out, v)
		}
	}
	return out
}
func groupsFor(n *model.Node) {
	n.GroupIDs = union(nil, n.ManualGroupIDs)
	keys := []string{}
	for k := range n.SourceGroups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n.GroupIDs = union(n.GroupIDs, n.SourceGroups[k])
	}
}
func pruneNodes(s *model.State, ids []string) {
	ns := []model.Node{}
	for _, n := range s.Nodes {
		if !contains(ids, n.ID) {
			ns = append(ns, n)
		}
	}
	s.Nodes = ns
	for i := range s.Pools {
		s.Pools[i].NodeIDs = without(s.Pools[i].NodeIDs, ids)
		s.Pools[i].ExcludeNodeIDs = without(s.Pools[i].ExcludeNodeIDs, ids)
	}
	for i := range s.Listeners {
		if s.Listeners[i].Kind == "node" && contains(ids, s.Listeners[i].TargetID) {
			s.Listeners[i].Enabled = false
		}
	}
	recount(s)
}
func recount(s *model.State) {
	for i := range s.Subscriptions {
		s.Subscriptions[i].NodeCount = 0
		for _, n := range s.Nodes {
			if contains(n.SourceIDs, s.Subscriptions[i].ID) {
				s.Subscriptions[i].NodeCount++
			}
		}
	}
}
func jsonMerge(v any, patch map[string]json.RawMessage) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	var m map[string]json.RawMessage
	if e = json.Unmarshal(b, &m); e != nil {
		return e
	}
	for k, v := range patch {
		m[k] = v
	}
	b, e = json.Marshal(m)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func filterPatch(p map[string]json.RawMessage, allowed ...string) error {
	for k := range p {
		if !contains(allowed, k) {
			return bad("不允许修改字段：" + k)
		}
	}
	return nil
}
func errorStatus(err error) (int, string, string) {
	var e *apiError
	if errors.As(err, &e) {
		return e.Status, e.Message, e.Code
	}
	return 500, "服务器处理失败，请查看运行日志", "internal"
}

// Preserve source attribution when a user updates effective group memberships.
func setNodeGroups(n *model.Node, desired []string) {
	inherited := []string{}
	for _, gs := range n.SourceGroups {
		inherited = union(inherited, gs)
	}
	manual := []string{}
	for _, g := range union(nil, desired) {
		if contains(n.ManualGroupIDs, g) || !contains(inherited, g) {
			manual = append(manual, g)
		}
	}
	n.ManualGroupIDs = manual
	for source, gs := range n.SourceGroups {
		kept := []string{}
		for _, g := range gs {
			if contains(desired, g) {
				kept = append(kept, g)
			}
		}
		n.SourceGroups[source] = kept
	}
	groupsFor(n)
}
