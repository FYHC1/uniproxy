package importer

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

// clashEntry keeps document-local labels separate from normalized native config.
// Invalid entries stay in the label index so they cannot be mistaken for absent
// definitions or let a second, ambiguous definition win by input order.
type clashEntry struct {
	line        int
	name        string
	node        model.ParsedNode
	err         error
	target      int
	depth       int
	fingerprint string
}

func parseClashNodes(root, list *yaml.Node, result *model.ParseResult) {
	entries := make([]clashEntry, len(list.Content))
	names := make(map[string][]int, len(entries))
	groups := clashGroupNames(root)

	// Pass one consumes and validates every native outbound, including entries
	// after the diagnostic detail cap. Only explicit names are referenceable;
	// generated display labels never create an implicit dependency target.
	for i, source := range list.Content {
		e := &entries[i]
		e.line, e.target = source.Line, -1
		e.name = yamlStringField(source, "name")
		if validDialerName(e.name) {
			names[e.name] = append(names[e.name], i)
		}
		value, err := nodeValue(source)
		if err != nil {
			e.err = err
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			e.err = errors.New("outbound entry must be an object")
			continue
		}
		var ref string
		if raw, present := obj["dialer-proxy"]; present {
			ref, ok = raw.(string)
			if !ok || !validDialerName(ref) {
				e.err = errors.New("dialer-proxy must be a non-empty valid same-document proxy name")
				continue
			}
			// This is deliberately a document-layer exception. The native
			// normalizer and every other import path still reject delegation.
			delete(obj, "dialer-proxy")
		}
		e.node, e.err = normalizeNative(obj)
		if e.err == nil {
			e.node.DialerProxy = ref
		}
	}

	// Duplicate explicit names are rejected even for equivalent configs. Unique
	// aliases with the same normalized identity remain valid and deduplicate by
	// fingerprint instead of choosing one definition of an ambiguous label.
	for i := range entries {
		e := &entries[i]
		if e.err == nil && len(names[e.name]) > 1 {
			e.err = errors.New("duplicate proxy name in the same document")
		}
	}

	// Pass two binds references only after the complete native name table is
	// available, allowing forward references without any external lookup.
	for i := range entries {
		e := &entries[i]
		ref := e.node.DialerProxy
		if e.err != nil || ref == "" {
			continue
		}
		switch {
		case builtinDialerName(ref):
			e.err = errors.New("dialer-proxy cannot reference a builtin outbound")
		case groups[ref]:
			e.err = errors.New("dialer-proxy cannot reference a proxy group; only same-document native proxies are supported")
		case len(names[ref]) == 0:
			e.err = errors.New("dialer-proxy target is missing from the same document")
		case len(names[ref]) != 1:
			e.err = errors.New("dialer-proxy target has an ambiguous duplicate proxy name")
		case names[ref][0] == i:
			e.err = errors.New("dialer-proxy cannot reference itself")
		default:
			e.target = names[ref][0]
		}
	}

	resolveClashGraph(entries)
	// Resolution is dependency-ordered internally, never in its public output.
	// Diagnostics also retain source order, regardless of forward references.
	for _, e := range entries {
		if e.err != nil {
			issue(result, e.line, e.err.Error())
		} else {
			result.Nodes = append(result.Nodes, e.node)
		}
	}
}

func validDialerName(name string) bool {
	return strings.TrimSpace(name) != "" && len(name) <= 1024 && utf8.ValidString(name) && !containsControl(name)
}

func builtinDialerName(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DIRECT", "REJECT", "REJECT-DROP", "GLOBAL", "PASS", "COMPATIBLE":
		return true
	}
	return false
}

func yamlStringField(object *yaml.Node, key string) string {
	if object.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i < len(object.Content); i += 2 {
		value := object.Content[i+1]
		if object.Content[i].Value == key && value.Kind == yaml.ScalarNode && value.Tag == "!!str" {
			return value.Value
		}
	}
	return ""
}

func clashGroupNames(root *yaml.Node) map[string]bool {
	groups := map[string]bool{}
	if root.Kind != yaml.MappingNode {
		return groups
	}
	for i := 0; i < len(root.Content); i += 2 {
		list := root.Content[i+1]
		if root.Content[i].Value != "proxy-groups" || list.Kind != yaml.SequenceNode {
			continue
		}
		for _, group := range list.Content {
			if name := yamlStringField(group, "name"); validDialerName(name) {
				groups[name] = true
			}
		}
	}
	return groups
}

// A functional graph (at most one upstream per node) can be walked iteratively.
// This finds even long cycles without unbounded recursion, then computes depth
// and recursive identity from the tail up. Rejecting a chain over 16 edges must
// not poison an otherwise valid suffix or depend on the document's entry order.
func resolveClashGraph(entries []clashEntry) {
	state := make([]uint8, len(entries)) // 0 unseen, 1 on current path, 2 resolved
	path := make([]int, 0, len(entries))
	for start := range entries {
		if state[start] == 2 {
			continue
		}
		path = path[:0]
		for current := start; ; current = entries[current].target {
			if state[current] == 2 {
				break
			}
			if state[current] == 1 {
				for j := len(path) - 1; j >= 0; j-- {
					entries[path[j]].err = errors.New("dialer-proxy dependency cycle is not allowed")
					if path[j] == current {
						break
					}
				}
				break
			}
			state[current] = 1
			path = append(path, current)
			if entries[current].err != nil || entries[current].target < 0 {
				break
			}
		}
		for j := len(path) - 1; j >= 0; j-- {
			index := path[j]
			e := &entries[index]
			if e.err == nil && e.target >= 0 {
				upstream := &entries[e.target]
				switch {
				case upstream.err != nil:
					e.err = errors.New("dialer-proxy target or its dependency is invalid")
				case upstream.depth >= MaxDialerDepth:
					e.err = errors.New("dialer-proxy chain exceeds the 16 dependency edge limit")
				default:
					e.depth = upstream.depth + 1
					e.node.DialerFingerprint = upstream.fingerprint
				}
			}
			if e.err == nil {
				e.fingerprint = Fingerprint(e.node)
				if e.fingerprint == "" {
					e.err = errors.New("invalid resolved proxy identity")
				}
			}
			state[index] = 2
		}
	}
}
