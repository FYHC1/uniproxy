package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/safehttp"
)

type importRequest struct {
	Mode            string   `json:"mode"`
	Content         string   `json:"content"`
	GroupIDs        []string `json:"groupIds"`
	GroupNames      []string `json:"groupNames"`
	Name            string   `json:"name"`
	IntervalMinutes int      `json:"intervalMinutes"`
	Preview         bool     `json:"preview"`
}
type fetchedImport struct {
	URL    string
	Result model.ParseResult
}

// Parser diagnostics are secret-safe by contract. Report a bounded first reason
// rather than echoing downloaded content or silently importing an incomplete source.
func subscriptionParseError(pr model.ParseResult, refresh bool) string {
	prefix := "订阅已下载，但配置解析不完整，未导入"
	if refresh {
		prefix = "订阅已下载，但配置解析不完整，已保留原节点"
	}
	total := pr.TotalErrors
	if total < len(pr.Errors) {
		total = len(pr.Errors)
	}
	if len(pr.Errors) == 0 {
		return prefix
	}
	first := pr.Errors[0]
	return fmt.Sprintf("%s：共 %d 项错误；配置第 %d 项：%s", prefix, total, first.Line, first.Message)
}

func emptyImport() model.ImportResult {
	return model.ImportResult{Errors: []model.ImportIssue{}, SubscriptionIDs: []string{}}
}
func (a *App) importNodes(ctx context.Context, r importRequest) (any, error) {
	if r.Mode != "text" && r.Mode != "file" && r.Mode != "subscriptions" {
		return nil, bad("导入方式无效")
	}
	if len(r.Content) > 5*1024*1024 {
		return nil, bad("导入内容超过 5 MiB")
	}
	if strings.TrimSpace(r.Content) == "" {
		return nil, bad("导入内容不能为空")
	}
	// Reject unusable metadata before any network request, so a long name or bad
	// group label cannot turn a completed download into a late validation error.
	// The subscription name only applies to subscription imports.
	if r.Mode == "subscriptions" {
		if name := strings.TrimSpace(r.Name); name != "" && !nameOK(name) {
			return nil, nameError("订阅")
		}
	}
	for _, name := range r.GroupNames {
		if !nameOK(strings.TrimSpace(name)) {
			return nil, nameError("自定义分组")
		}
	}
	// Interval is subscription-only metadata; other modes keep ignoring it.
	if r.Mode == "subscriptions" && r.IntervalMinutes != 0 && (r.IntervalMinutes < 5 || r.IntervalMinutes > 10080) {
		return nil, bad("订阅更新间隔必须为 5–10080 分钟")
	}
	fetched := []fetchedImport{}
	combined := model.ParseResult{Nodes: []model.ParsedNode{}, Errors: []model.ImportIssue{}}
	if r.Mode == "subscriptions" {
		urls := strings.Fields(r.Content)
		if len(urls) > 50 {
			return nil, bad("单次最多导入 50 个订阅")
		}
		seen := map[string]bool{}
		for i, raw := range urls {
			if seen[raw] {
				continue
			}
			seen[raw] = true
			body, e := a.fetch(ctx, raw)
			if e != nil {
				combined.Errors = append(combined.Errors, model.ImportIssue{Line: i + 1, Message: e.Error()})
				continue
			}
			pr := importer.Parse(body)
			if len(pr.Errors) > 0 {
				combined.Errors = append(combined.Errors, model.ImportIssue{Line: i + 1, Message: subscriptionParseError(pr, false)})
				continue
			}
			if len(pr.Nodes) == 0 {
				combined.Errors = append(combined.Errors, model.ImportIssue{Line: i + 1, Message: "订阅没有有效节点，未导入"})
				continue
			}
			fetched = append(fetched, fetchedImport{raw, pr})
			combined.Nodes = append(combined.Nodes, pr.Nodes...)
		}
	} else {
		combined = importer.Parse(r.Content)
		fetched = append(fetched, fetchedImport{Result: combined})
	}
	if combined.TotalErrors < len(combined.Errors) {
		combined.TotalErrors = len(combined.Errors)
	}
	if r.Preview {
		for i := range combined.Nodes {
			combined.Nodes[i].Config = nil
		}
		return combined, nil
	}
	if len(combined.Nodes) == 0 {
		msg := "没有可导入的节点"
		if len(combined.Errors) > 0 {
			msg += "：" + combined.Errors[0].Message
		}
		return nil, bad(msg)
	}
	return a.mutate(ctx, "导入节点/订阅", func(s *model.State) (any, error) {
		groupIDs := union(nil, r.GroupIDs)
		for _, name := range r.GroupNames {
			name = strings.TrimSpace(name)
			if !nameOK(name) {
				return nil, bad("自定义分组名称无效")
			}
			id := ""
			for _, g := range s.Groups {
				if g.Name == name {
					id = g.ID
					break
				}
			}
			if id == "" {
				id = ID()
				s.Groups = append(s.Groups, model.Group{ID: id, Name: name, Color: "#0d9488", CreatedAt: model.Now()})
			}
			groupIDs = union(groupIDs, []string{id})
		}
		out := emptyImport()
		out.Errors = combined.Errors
		out.TotalErrors = combined.TotalErrors
		out.ErrorsTruncated = combined.ErrorsTruncated
		for _, f := range fetched {
			sourceID := ""
			if f.URL != "" {
				for _, sub := range s.Subscriptions {
					if sub.URL == f.URL {
						return nil, bad("订阅已存在，请使用刷新功能")
					}
				}
				sourceID = ID()
				name := strings.TrimSpace(r.Name)
				if name == "" {
					u, _ := url.Parse(f.URL)
					// A legal hostname can exceed the display-name ceiling.
					name = truncateRunes(u.Hostname(), maxNameRunes)
				}
				interval := r.IntervalMinutes
				if interval == 0 {
					interval = 60
				}
				now := model.Now()
				s.Subscriptions = append(s.Subscriptions, model.Subscription{ID: sourceID, Name: name, URL: f.URL, GroupIDs: groupIDs, Enabled: true, IntervalMinutes: interval, LastFetchedAt: now, CreatedAt: now, UpdatedAt: now})
				out.SubscriptionIDs = append(out.SubscriptionIDs, sourceID)
			}
			if err := addParsed(s, f.Result.Nodes, groupIDs, sourceID, &out); err != nil {
				return nil, err
			}
		}
		recount(s)
		return out, nil
	})
}
func addParsed(s *model.State, parsed []model.ParsedNode, groupIDs []string, sourceID string, out *model.ImportResult) error {
	// Reserve every identity first: forward references and equivalent relay aliases
	// bind to the same stable node, regardless of source order or display names.
	fpToID := map[string]string{}
	inDocument := map[string]bool{}
	for _, n := range s.Nodes {
		fpToID[n.Fingerprint] = n.ID
	}
	fingerprints := make([]string, len(parsed))
	for i, p := range parsed {
		fp := importer.Fingerprint(p)
		if fp == "" {
			return bad("节点配置指纹无效")
		}
		fingerprints[i] = fp
		inDocument[fp] = true
		if fpToID[fp] == "" {
			fpToID[fp] = ID()
		}
	}
	for _, p := range parsed {
		if p.DialerProxy != "" && (p.DialerFingerprint == "" || !inDocument[p.DialerFingerprint]) {
			return bad("中转依赖未在本次导入中完整解析")
		}
		if p.DialerFingerprint != "" && !inDocument[p.DialerFingerprint] {
			return bad("中转依赖必须属于同一导入文件")
		}
	}
	for i, p := range parsed {
		fp := fingerprints[i]
		id := fpToID[fp]
		idx := nodeIndex(s, id)
		now := model.Now()
		dialerID := ""
		if p.DialerFingerprint != "" {
			dialerID = fpToID[p.DialerFingerprint]
		}
		upstreamIdentity := p.DialerFingerprint
		p.DialerProxy = ""
		p.DialerFingerprint = ""
		if idx < 0 {
			n := model.Node{ParsedNode: p, ID: id, Fingerprint: fp, DialerID: dialerID, Enabled: true, Manual: sourceID == "", GroupIDs: []string{}, ManualGroupIDs: []string{}, SourceIDs: []string{}, SourceGroups: map[string][]string{}, Status: "unknown", CreatedAt: now, UpdatedAt: now}
			s.Nodes = append(s.Nodes, n)
			idx = len(s.Nodes) - 1
			out.Added++
		} else {
			out.Skipped++
		}
		n := &s.Nodes[idx]
		n.DialerID = dialerID
		n.DialerIdentity = upstreamIdentity
		n.DialerName = ""
		n.DialerError = ""
		if sourceID == "" {
			n.Manual = true
			n.ManualGroupIDs = union(n.ManualGroupIDs, groupIDs)
		} else {
			n.SourceIDs = union(n.SourceIDs, []string{sourceID})
			if n.SourceGroups == nil {
				n.SourceGroups = map[string][]string{}
			}
			n.SourceGroups[sourceID] = union(nil, groupIDs)
		}
		groupsFor(n)
		n.UpdatedAt = now
	}
	return nil
}
func reconcile(s *model.State, subID string, pr model.ParseResult, force bool) (model.ImportResult, error) {
	out := emptyImport()
	idx := -1
	for i, v := range s.Subscriptions {
		if v.ID == subID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return out, missing()
	}
	if len(pr.Errors) > 0 {
		return out, bad(subscriptionParseError(pr, true))
	}
	sub := &s.Subscriptions[idx]
	old := 0
	removed := 0
	fps := map[string]bool{}
	for _, p := range pr.Nodes {
		fps[importer.Fingerprint(p)] = true
	}
	for _, n := range s.Nodes {
		if contains(n.SourceIDs, subID) {
			old++
			if !fps[n.Fingerprint] {
				removed++
			}
		}
	}
	if !force && (len(pr.Nodes) == 0 || (old > 0 && removed*2 > old)) {
		return out, &apiError{409, "订阅为空或超过一半节点将被删除；已保留原节点，请确认后强制刷新", "subscription_shrink"}
	}
	if err := addParsed(s, pr.Nodes, sub.GroupIDs, subID, &out); err != nil {
		return out, err
	}
	names := map[string]string{}
	for _, p := range pr.Nodes {
		names[importer.Fingerprint(p)] = p.Name
	}
	orphans := []string{}
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if !contains(n.SourceIDs, subID) {
			continue
		}
		if !fps[n.Fingerprint] {
			n.SourceIDs = without(n.SourceIDs, []string{subID})
			delete(n.SourceGroups, subID)
			groupsFor(n)
			if !n.Manual && len(n.SourceIDs) == 0 {
				orphans = append(orphans, n.ID)
			}
		} else {
			if !n.CustomName {
				n.Name = names[n.Fingerprint]
			}
			n.UpdatedAt = model.Now()
			out.Updated++
		}
	}
	out.Removed = len(orphans)
	pruneNodes(s, orphans)
	sub = &s.Subscriptions[idx]
	sub.LastFetchedAt = model.Now()
	sub.LastError = ""
	sub.UpdatedAt = model.Now()
	recount(s)
	return out, nil
}
func (a *App) fetch(ctx context.Context, raw string) (string, error) {
	return safehttp.Fetch(ctx, a.client, raw)
}
func (a *App) refresh(ctx context.Context, id string, force bool) (any, error) {
	a.mu.Lock()
	if a.refreshing[id] {
		a.mu.Unlock()
		return nil, &apiError{409, "订阅正在刷新", "busy"}
	}
	s := a.store.Snapshot()
	var sub *model.Subscription
	for i := range s.Subscriptions {
		if s.Subscriptions[i].ID == id {
			sub = &s.Subscriptions[i]
			break
		}
	}
	if sub == nil {
		a.mu.Unlock()
		return nil, missing()
	}
	a.refreshing[id] = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); delete(a.refreshing, id); a.mu.Unlock() }()
	body, e := a.fetch(ctx, sub.URL)
	var result any
	if e == nil {
		pr := importer.Parse(body)
		if strings.TrimSpace(body) == "" {
			pr = model.ParseResult{Nodes: []model.ParsedNode{}, Errors: []model.ImportIssue{}}
		} // still requires explicit shrink confirmation
		result, e = a.mutate(ctx, "刷新订阅："+sub.Name, func(current *model.State) (any, error) {
			for _, latest := range current.Subscriptions {
				if latest.ID == id {
					if latest.URL != sub.URL || latest.UpdatedAt != sub.UpdatedAt {
						return nil, &apiError{409, "订阅配置在下载期间发生变化，请重新刷新", "stale_subscription"}
					}
					return reconcile(current, id, pr, force)
				}
			}
			return nil, missing()
		})
	}
	if e != nil {
		a.mu.Lock()
		latest := a.store.Snapshot()
		for i := range latest.Subscriptions {
			if latest.Subscriptions[i].ID == id && latest.Subscriptions[i].URL == sub.URL && latest.Subscriptions[i].UpdatedAt == sub.UpdatedAt {
				latest.Subscriptions[i].LastError = e.Error()
				latest.Subscriptions[i].LastFetchedAt = model.Now()
			}
		}
		_ = a.store.Save(latest)
		a.store.Activity("warning", "订阅刷新失败："+e.Error())
		a.mu.Unlock()
		var ae *apiError
		if errors.As(e, &ae) {
			return nil, e
		}
		return nil, bad(e.Error())
	}
	return result, nil
}
func (a *App) subscriptionPatch(ctx context.Context, id string, patch map[string]json.RawMessage) (any, error) {
	if e := filterPatch(patch, "name", "groupIds", "enabled", "intervalMinutes"); e != nil {
		return nil, e
	}
	return a.mutate(ctx, "更新订阅", func(s *model.State) (any, error) {
		for i := range s.Subscriptions {
			if s.Subscriptions[i].ID == id {
				if e := jsonMerge(&s.Subscriptions[i], patch); e != nil {
					return nil, bad("订阅字段格式无效")
				}
				sub := &s.Subscriptions[i]
				sub.UpdatedAt = model.Now()
				for j := range s.Nodes {
					n := &s.Nodes[j]
					if contains(n.SourceIDs, id) {
						n.SourceGroups[id] = union(nil, sub.GroupIDs)
						groupsFor(n)
					}
				}
				public := *sub
				public.URL = "[订阅地址已隐藏]"
				return public, nil
			}
		}
		return nil, missing()
	})
}
func (a *App) scheduler() {
	defer a.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.background.Done():
			return
		case <-ticker.C:
			s := a.store.Snapshot()
			now := time.Now()
			for _, sub := range s.Subscriptions {
				if !sub.Enabled {
					continue
				}
				last, _ := time.Parse(time.RFC3339Nano, sub.LastFetchedAt)
				if now.Sub(last) >= time.Duration(sub.IntervalMinutes)*time.Minute {
					_, _ = a.refresh(a.background, sub.ID, false)
					if a.background.Err() != nil {
						return
					}
				}
			}
			if now.Sub(a.lastHealth) >= time.Duration(s.Settings.HealthIntervalSeconds)*time.Second && a.core.Status().Running {
				ids := []string{}
				blocked := model.RelayErrors(s.Nodes)
				for _, n := range s.Nodes {
					if n.Enabled && blocked[n.ID] == "" {
						ids = append(ids, n.ID)
					}
				}
				if len(ids) > 0 {
					_, _ = a.startJob(ids, "latency")
				}
				a.lastHealth = now
			}
		}
	}
}
