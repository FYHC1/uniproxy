package model

import "errors"

const MaxRelayDepth = 16

// ValidateRelayGraph validates persistent IDs, never user-controlled native names.
// A missing target is permitted in stored state: deleting a relay must leave its
// dependents visibly blocked, not rewrite them into directly dialed outbounds.
func ValidateRelayGraph(nodes []Node) error {
	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for _, start := range nodes {
		seen := map[string]bool{}
		current := start
		depth := 0
		for current.DialerID != "" {
			if seen[current.ID] {
				return errors.New("中转依赖存在循环，不能应用")
			}
			seen[current.ID] = true
			if current.DialerID == current.ID {
				return errors.New("节点不能以自身作为中转")
			}
			depth++
			if depth > MaxRelayDepth {
				return errors.New("中转链超过16层限制")
			}
			next, ok := byID[current.DialerID]
			if !ok {
				break
			}
			current = next
		}
	}
	return nil
}

// RelayErrors returns safe public errors for nodes whose chain cannot be used.
// It does not report mere unknown health as failure and never silently removes a
// dependency. The compiler and public UI use the same configuration predicate.
func RelayErrors(nodes []Node) map[string]string {
	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	errorsByID := map[string]string{}
	for _, start := range nodes {
		if start.DialerID == "" {
			continue
		}
		seen := map[string]bool{}
		current := start
		depth := 0
		for current.DialerID != "" {
			if seen[current.ID] {
				errorsByID[start.ID] = "中转依赖存在循环"
				break
			}
			seen[current.ID] = true
			depth++
			if depth > MaxRelayDepth {
				errorsByID[start.ID] = "中转链超过16层限制"
				break
			}
			next, ok := byID[current.DialerID]
			if !ok {
				errorsByID[start.ID] = "中转节点已删除或缺失，禁止直连回退"
				break
			}
			if !next.Enabled {
				errorsByID[start.ID] = "中转节点已禁用，禁止直连回退"
				break
			}
			current = next
		}
	}
	return errorsByID
}
