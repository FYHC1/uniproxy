package model

import "time"

type ParsedNode struct {
	Name     string         `json:"name"`
	Protocol string         `json:"protocol"`
	Server   string         `json:"server"`
	Port     int            `json:"port"`
	UDP      bool           `json:"udp"`
	Config   map[string]any `json:"config,omitempty"`
	// Parsed-only same-document dependency; never passed through in Config.
	DialerProxy       string `json:"dialerProxy,omitempty"`
	DialerFingerprint string `json:"-"`
}
type ImportIssue struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}
type ParseResult struct {
	Nodes           []ParsedNode  `json:"nodes"`
	Errors          []ImportIssue `json:"errors"`
	TotalErrors     int           `json:"totalErrors"`
	ErrorsTruncated bool          `json:"errorsTruncated"`
}
type Node struct {
	ParsedNode
	ID             string              `json:"id"`
	Fingerprint    string              `json:"fingerprint,omitempty"`
	DialerID       string              `json:"dialerId,omitempty"`
	DialerIdentity string              `json:"dialerIdentity,omitempty"` // last validated upstream fingerprint, retained if relay is deleted
	DialerName     string              `json:"dialerName,omitempty"`
	DialerError    string              `json:"dialerError,omitempty"`
	Enabled        bool                `json:"enabled"`
	Manual         bool                `json:"manual"`
	CustomName     bool                `json:"customName"`
	GroupIDs       []string            `json:"groupIds"`
	ManualGroupIDs []string            `json:"manualGroupIds,omitempty"`
	SourceGroups   map[string][]string `json:"sourceGroups,omitempty"`
	SourceIDs      []string            `json:"sourceIds"`
	Status         string              `json:"status"`
	LatencyMS      *int64              `json:"latencyMs"`
	SpeedMbps      *float64            `json:"speedMbps"`
	LastCheckedAt  string              `json:"lastCheckedAt"`
	LastError      string              `json:"lastError"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
}
type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"createdAt"`
}
type Pool struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Strategy        string   `json:"strategy"`
	HashKey         string   `json:"hashKey"`
	NodeIDs         []string `json:"nodeIds"`
	GroupIDs        []string `json:"groupIds"`
	GroupMode       string   `json:"groupMode"`
	ExcludeNodeIDs  []string `json:"excludeNodeIds"`
	ExcludeGroupIDs []string `json:"excludeGroupIds"`
	Enabled         bool     `json:"enabled"`
	UDP             bool     `json:"udp"`
	TestURL         string   `json:"testUrl"`
	IntervalSeconds int      `json:"intervalSeconds"`
	TimeoutMS       int      `json:"timeoutMs"`
	ToleranceMS     int      `json:"toleranceMs"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}
type Credential struct {
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	HasPassword bool   `json:"hasPassword,omitempty"`
}
type Listener struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Kind      string       `json:"kind"`
	TargetID  string       `json:"targetId"`
	Protocol  string       `json:"protocol"`
	Host      string       `json:"host"`
	Port      int          `json:"port"`
	UDP       bool         `json:"udp"`
	Enabled   bool         `json:"enabled"`
	Users     []Credential `json:"users"`
	CreatedAt string       `json:"createdAt"`
	UpdatedAt string       `json:"updatedAt"`
}
type Subscription struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	URL             string   `json:"url"`
	GroupIDs        []string `json:"groupIds"`
	Enabled         bool     `json:"enabled"`
	IntervalMinutes int      `json:"intervalMinutes"`
	LastFetchedAt   string   `json:"lastFetchedAt"`
	LastError       string   `json:"lastError"`
	NodeCount       int      `json:"nodeCount"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}
type Settings struct {
	TestURL               string `json:"testUrl"`
	SpeedTestURL          string `json:"speedTestUrl"`
	TestTimeoutMS         int    `json:"testTimeoutMs"`
	TestConcurrency       int    `json:"testConcurrency"`
	SpeedTestBytes        int64  `json:"speedTestBytes"`
	HealthIntervalSeconds int    `json:"healthIntervalSeconds"`
	PortRangeStart        int    `json:"portRangeStart"`
	PortRangeEnd          int    `json:"portRangeEnd"`
}
type State struct {
	Nodes         []Node         `json:"nodes"`
	Groups        []Group        `json:"groups"`
	Pools         []Pool         `json:"pools"`
	Listeners     []Listener     `json:"listeners"`
	Subscriptions []Subscription `json:"subscriptions"`
	Settings      Settings       `json:"settings"`
	Revision      int64          `json:"revision"`
}
type RuntimeStatus struct {
	Available       bool              `json:"available"`
	Running         bool              `json:"running"`
	Applying        bool              `json:"applying"`
	Version         string            `json:"version"`
	AppliedRevision int64             `json:"appliedRevision"`
	LastError       string            `json:"lastError"`
	StartedAt       string            `json:"startedAt"`
	Dirty           bool              `json:"dirty"`
	Listeners       []ListenerRuntime `json:"listeners"`
}
type ListenerRuntime struct {
	ID        string `json:"id"`
	Listening bool   `json:"listening"`
	Error     string `json:"error"`
}
type CheckResult struct {
	NodeID    string   `json:"nodeId"`
	OK        bool     `json:"ok"`
	LatencyMS *int64   `json:"latencyMs"`
	SpeedMbps *float64 `json:"speedMbps"`
	Bytes     int64    `json:"bytes"`
	Error     string   `json:"error"`
}
type Job struct {
	ID         string        `json:"id"`
	Kind       string        `json:"kind"`
	Status     string        `json:"status"`
	Total      int           `json:"total"`
	Completed  int           `json:"completed"`
	Results    []CheckResult `json:"results"`
	CreatedAt  string        `json:"createdAt"`
	FinishedAt string        `json:"finishedAt"`
}
type Activity struct {
	ID        string `json:"id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}
type ImportResult struct {
	Added           int           `json:"added"`
	Updated         int           `json:"updated"`
	Removed         int           `json:"removed"`
	Skipped         int           `json:"skipped"`
	Errors          []ImportIssue `json:"errors"`
	SubscriptionIDs []string      `json:"subscriptionIds"`
	TotalErrors     int           `json:"totalErrors"`
	ErrorsTruncated bool          `json:"errorsTruncated"`
}

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func DefaultState() State {
	return State{Nodes: []Node{}, Groups: []Group{}, Pools: []Pool{}, Listeners: []Listener{}, Subscriptions: []Subscription{}, Settings: Settings{TestURL: "https://www.gstatic.com/generate_204", SpeedTestURL: "https://speed.cloudflare.com/__down?bytes=10485760", TestTimeoutMS: 8000, TestConcurrency: 8, SpeedTestBytes: 10485760, HealthIntervalSeconds: 300, PortRangeStart: 20000, PortRangeEnd: 20999}}
}

// PoolMembers resolves stable, ordered membership. Health remains a core decision, not a stale DB filter.
func PoolMembers(state State, pool Pool) []Node {
	out := []Node{}
	blocked := RelayErrors(state.Nodes)
	if !pool.Enabled {
		return out
	}
	contains := func(xs []string, v string) bool {
		for _, x := range xs {
			if x == v {
				return true
			}
		}
		return false
	}
	for _, n := range state.Nodes {
		if !n.Enabled || blocked[n.ID] != "" || (pool.UDP && !n.UDP) || contains(pool.ExcludeNodeIDs, n.ID) {
			continue
		}
		excluded := false
		for _, g := range n.GroupIDs {
			if contains(pool.ExcludeGroupIDs, g) {
				excluded = true
			}
		}
		if excluded {
			continue
		}
		selected := contains(pool.NodeIDs, n.ID)
		if len(pool.GroupIDs) > 0 {
			matches := 0
			for _, g := range pool.GroupIDs {
				if contains(n.GroupIDs, g) {
					matches++
				}
			}
			selected = selected || (pool.GroupMode == "all" && matches == len(pool.GroupIDs)) || (pool.GroupMode != "all" && matches > 0)
		}
		if selected {
			out = append(out, n)
		}
	}
	return out
}
