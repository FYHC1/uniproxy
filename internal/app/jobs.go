package app

import (
	"context"
	"encoding/json"
	"github.com/uniproxy/uniproxy/internal/model"
	"sort"
	"sync"
	"time"
)

func cloneJob(j *model.Job) model.Job {
	b, _ := json.Marshal(j)
	var out model.Job
	_ = json.Unmarshal(b, &out)
	if out.Results == nil {
		out.Results = []model.CheckResult{}
	}
	return out
}
func (a *App) jobList() []model.Job {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	out := []model.Job{}
	for _, j := range a.jobs {
		out = append(out, cloneJob(j))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}
func (a *App) startJob(ids []string, kind string) (model.Job, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	empty := model.Job{}
	if a.closed || a.background.Err() != nil {
		return empty, &apiError{503, "管理服务正在关闭", "shutting_down"}
	}
	if kind != "latency" && kind != "speed" {
		return empty, bad("检测类型无效")
	}
	ids = union(nil, ids)
	if len(ids) == 0 || len(ids) > 5000 {
		return empty, bad("请选择 1–5000 个节点")
	}
	if !a.core.Status().Running {
		return empty, &apiError{409, "代理内核未运行，请安装内核并应用配置后检测", "core_unavailable"}
	}
	state := a.store.Snapshot()
	if status := a.core.Status(); status.Dirty || status.Applying || status.AppliedRevision != state.Revision || a.applyError != "" {
		return empty, &apiError{409, "配置尚未成功应用，暂不能检测节点", "runtime_dirty"}
	}
	blocked := model.RelayErrors(state.Nodes)
	for _, id := range ids {
		if blocked[id] != "" {
			return empty, bad("节点中转不可用，请先恢复中转依赖后再检测")
		}
		i := nodeIndex(&state, id)
		if i < 0 {
			return empty, missing()
		}
		if !state.Nodes[i].Enabled {
			return empty, bad("禁用节点不能检测，请先启用")
		}
	}
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	for _, j := range a.jobs {
		if j.Status == "running" || j.Status == "queued" {
			return empty, &apiError{409, "已有检测任务运行，请等待完成或取消", "job_busy"}
		}
	}
	if len(a.jobs) >= 30 {
		old := ""
		stamp := ""
		for id, j := range a.jobs {
			if stamp == "" || j.CreatedAt < stamp {
				old = id
				stamp = j.CreatedAt
			}
		}
		delete(a.jobs, old)
		delete(a.cancels, old)
	}
	j := &model.Job{ID: ID(), Kind: kind, Status: "queued", Total: len(ids), Results: []model.CheckResult{}, CreatedAt: model.Now()}
	a.jobs[j.ID] = j
	ctx, cancel := context.WithCancel(a.background)
	a.cancels[j.ID] = cancel
	a.wg.Add(1)
	go a.runJob(ctx, j.ID, ids, kind, state.Settings, state.Revision)
	return cloneJob(j), nil
}
func (a *App) runJob(ctx context.Context, id string, ids []string, kind string, settings model.Settings, revision int64) {
	defer a.wg.Done()
	a.jobMu.Lock()
	j := a.jobs[id]
	j.Status = "running"
	a.jobMu.Unlock()
	limit := settings.TestConcurrency
	if kind == "speed" && limit > 2 {
		limit = 2
	}
	queue := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for nodeID := range queue {
				if ctx.Err() != nil {
					return
				}
				s := a.store.Snapshot()
				status := a.core.Status()
				if s.Revision != revision || status.AppliedRevision != revision || status.Applying || !status.Running {
					a.recordResult(id, model.CheckResult{NodeID: nodeID, Error: "配置已变化，跳过旧版本检测"}, kind)
					continue
				}
				idx := nodeIndex(&s, nodeID)
				if idx < 0 || !s.Nodes[idx].Enabled {
					a.recordResult(id, model.CheckResult{NodeID: nodeID, Error: "节点已删除或禁用"}, kind)
					continue
				}
				fingerprint := s.Nodes[idx].Fingerprint
				var result model.CheckResult
				var err error
				if kind == "speed" {
					result, err = a.core.Speed(ctx, nodeID, settings.SpeedTestURL, 30*time.Second, settings.SpeedTestBytes)
				} else {
					result, err = a.core.Check(ctx, nodeID, settings.TestURL, time.Duration(settings.TestTimeoutMS)*time.Millisecond)
				}
				result.NodeID = nodeID
				if err != nil {
					result.OK = false
					result.Error = err.Error()
				}
				if ctx.Err() != nil {
					return
				}
				a.mu.Lock()
				current := a.store.Snapshot()
				ni := nodeIndex(&current, nodeID)
				currentStatus := a.core.Status()
				if current.Revision != revision || currentStatus.AppliedRevision != revision || currentStatus.Applying || !currentStatus.Running {
					a.mu.Unlock()
					a.recordResult(id, model.CheckResult{NodeID: nodeID, Error: "配置已变化，丢弃旧版本检测结果"}, kind)
					continue
				}
				if ni >= 0 && current.Nodes[ni].Enabled && current.Nodes[ni].Fingerprint == fingerprint {
					n := &current.Nodes[ni]
					n.LastCheckedAt = model.Now()
					n.LastError = result.Error
					if result.OK {
						n.Status = "healthy"
						if result.LatencyMS != nil {
							n.LatencyMS = result.LatencyMS
						}
						if kind == "speed" {
							n.SpeedMbps = result.SpeedMbps
						}
					} else {
						n.Status = "unhealthy"
						n.LatencyMS = nil
						if kind == "speed" {
							n.SpeedMbps = nil
						}
					}
					_ = a.store.Save(current)
				}
				a.mu.Unlock()
				a.recordResult(id, result, kind)
			}
		}()
	}
send:
	for _, nodeID := range ids {
		select {
		case <-ctx.Done():
			break send
		case queue <- nodeID:
		}
	}
	close(queue)
	wg.Wait()
	a.jobMu.Lock()
	j = a.jobs[id]
	j.Status = "completed"
	if ctx.Err() != nil {
		j.Status = "cancelled"
	}
	j.FinishedAt = model.Now()
	if cancel := a.cancels[id]; cancel != nil {
		cancel()
	}
	delete(a.cancels, id)
	a.jobMu.Unlock()
	a.store.Activity("info", "节点"+map[string]string{"latency": "连通性检测", "speed": "下载测速"}[kind]+"任务结束")
}
func (a *App) recordResult(id string, r model.CheckResult, kind string) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	j := a.jobs[id]
	j.Results = append(j.Results, r)
	j.Completed++
}
func (a *App) getJob(id string) (model.Job, error) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	j := a.jobs[id]
	if j == nil {
		return model.Job{}, missing()
	}
	return cloneJob(j), nil
}
func (a *App) cancelJob(id string) (any, error) {
	a.jobMu.Lock()
	j := a.jobs[id]
	if j == nil {
		a.jobMu.Unlock()
		return nil, missing()
	}
	if cancel := a.cancels[id]; cancel != nil {
		cancel()
	}
	copy := cloneJob(j)
	a.jobMu.Unlock()
	return map[string]any{"data": copy, "runtime": a.Runtime()}, nil
}
