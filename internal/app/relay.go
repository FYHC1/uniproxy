package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"strings"
)

func annotateRelays(s *model.State) {
	byID := map[string]string{}
	for _, n := range s.Nodes {
		byID[n.ID] = n.Name
	}
	blocked := model.RelayErrors(s.Nodes)
	for i := range s.Nodes {
		n := &s.Nodes[i]
		n.DialerProxy = ""
		n.DialerFingerprint = ""
		n.DialerName = ""
		n.DialerError = ""
		if n.DialerID != "" {
			n.DialerName = byID[n.DialerID]
			n.DialerError = blocked[n.ID]
		}
	}
}

// recomputeFingerprints sanitizes each outbound using the offline parser, then
// resolves identity by stable graph IDs. Raw configs still cannot supply dialers.
func recomputeFingerprints(s *model.State) error {
	if e := model.ValidateRelayGraph(s.Nodes); e != nil {
		return bad(e.Error())
	}
	byID := map[string]int{}
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if _, exists := byID[n.ID]; exists {
			return bad("备份包含重复节点 ID")
		}
		byID[n.ID] = i
		if n.Config == nil {
			return bad("备份缺少节点配置")
		}
		raw := map[string]any{}
		for k, v := range n.Config {
			raw[k] = v
		}
		raw["name"] = n.Name
		b, e := json.Marshal(map[string]any{"proxies": []any{raw}})
		if e != nil {
			return bad("备份节点配置无效")
		}
		pr := importer.Parse(string(b))
		if len(pr.Errors) > 0 || len(pr.Nodes) != 1 || pr.Nodes[0].DialerProxy != "" {
			return bad("备份包含不安全或无效的节点配置")
		}
		n.ParsedNode = pr.Nodes[0]
		n.DialerName = ""
		n.DialerError = ""
	}
	done := map[string]bool{}
	var visit func(string, int) (string, error)
	visit = func(id string, depth int) (string, error) {
		if depth > model.MaxRelayDepth {
			return "", errors.New("中转链超过16层")
		}
		i, ok := byID[id]
		if !ok {
			return "", errors.New("missing relay")
		}
		n := &s.Nodes[i]
		if done[id] {
			return n.Fingerprint, nil
		}
		p := n.ParsedNode
		if n.DialerID != "" {
			if _, ok := byID[n.DialerID]; !ok { // retained deleted target: stable nonempty sentinel cannot collapse into a direct node
				if validRelayIdentity(n.DialerIdentity) {
					p.DialerFingerprint = n.DialerIdentity
				} else {
					sum := sha256.Sum256([]byte("uniproxy-missing-relay:" + n.DialerID))
					p.DialerFingerprint = hex.EncodeToString(sum[:])
				}
			} else {
				fp, e := visit(n.DialerID, depth+1)
				if e != nil {
					return "", e
				}
				p.DialerFingerprint = fp
			}
		}
		n.DialerIdentity = p.DialerFingerprint
		n.Fingerprint = importer.Fingerprint(p)
		if n.Fingerprint == "" {
			return "", errors.New("invalid fingerprint")
		}
		done[id] = true
		return n.Fingerprint, nil
	}
	for _, n := range s.Nodes {
		if _, e := visit(n.ID, 0); e != nil {
			return bad("备份中转关系无效")
		}
	}
	return nil
}

// Export names use stable IDs to disambiguate same-named nodes and preserve
// references across edit/import cycles. Native policy and controller are omitted.
func exportNativeNodes(s model.State) []map[string]any {
	names := map[string]string{}
	counts := map[string]int{}
	used := map[string]bool{}
	for _, n := range s.Nodes {
		counts[n.Name]++
	}
	for _, n := range s.Nodes {
		if counts[n.Name] == 1 && !reservedExportName(n.Name) && !used[n.Name] {
			names[n.ID] = n.Name
			used[n.Name] = true
		}
	}
	for _, n := range s.Nodes {
		if names[n.ID] == "" {
			candidate := "n-" + n.ID
			for used[candidate] {
				candidate = "_" + candidate
			}
			names[n.ID] = candidate
			used[candidate] = true
		}
	}
	missingNames := map[string]string{}
	for _, n := range s.Nodes {
		if n.DialerID != "" && names[n.DialerID] == "" && missingNames[n.DialerID] == "" {
			candidate := "missing-" + n.DialerID
			for used[candidate] {
				candidate = "_" + candidate
			}
			missingNames[n.DialerID] = candidate
			used[candidate] = true
		}
	}
	ps := []map[string]any{}
	for _, n := range s.Nodes {
		m := map[string]any{}
		for k, v := range n.Config {
			m[k] = v
		}
		m["name"] = names[n.ID]
		if n.DialerID != "" {
			target := names[n.DialerID]
			if target == "" {
				target = missingNames[n.DialerID]
			}
			m["dialer-proxy"] = target
		}
		ps = append(ps, m)
	}
	return ps
}
func reservedExportName(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DIRECT", "REJECT", "REJECT-DROP", "GLOBAL", "PASS", "COMPATIBLE":
		return true
	}
	return false
}
func validRelayIdentity(value string) bool {
	b, e := hex.DecodeString(value)
	return e == nil && len(b) == sha256.Size
}
