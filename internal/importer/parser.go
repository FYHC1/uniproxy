// Package importer converts untrusted, offline subscription text to native Mihomo
// outbound maps. It never resolves hosts, fetches URLs, opens files, or runs code.
package importer

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

const (
	MaxContentBytes   = 5 * 1024 * 1024
	MaxNodes          = 5000
	MaxDialerDepth    = 16 // Maximum same-document dependency edges.
	maxDepth          = 32
	maxDocumentValues = 250000
	maxBase64Depth    = 4
	maxIssues         = 128
	// MaxNodeNameRunes matches the management API display-name ceiling. Imported
	// labels are presentation only (identity is the fingerprint), so a longer
	// upstream label is shortened instead of dropping an otherwise valid node.
	MaxNodeNameRunes = 160
	// maxNodeNameRunes rejects implausible labels outright; it is a malformed
	// input guard, not the storage limit.
	maxNodeNameRunes = 1024
)

// Parse accepts URI lists, base64 subscriptions, Clash YAML/JSON, or sing-box
// JSON. Valid entries survive invalid siblings, even beyond the error detail
// cap; callers must inspect TotalErrors before applying a subscription refresh.
// Issues contain no original values. Clash references resolve only within the
// same document; normalized native Config never carries dialer-proxy.
func Parse(content string) model.ParseResult {
	result := model.ParseResult{Nodes: []model.ParsedNode{}, Errors: []model.ImportIssue{}}
	parseContent(content, 0, &result)
	return result
}

func issue(result *model.ParseResult, line int, message string) {
	if line < 1 {
		line = 1
	}
	result.TotalErrors++
	if len(result.Errors) < maxIssues {
		result.Errors = append(result.Errors, model.ImportIssue{Line: line, Message: message})
	} else {
		result.ErrorsTruncated = true
	}
}

func parseContent(content string, depth int, result *model.ParseResult) {
	if len(content) > MaxContentBytes {
		issue(result, 1, "import exceeds the 5 MiB limit")
		return
	}
	if !utf8.ValidString(content) {
		issue(result, 1, "import must contain valid UTF-8 text")
		return
	}
	content = strings.TrimPrefix(content, "\ufeff")
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		issue(result, 1, "import is empty")
		return
	}
	first := firstDataLine(trimmed)
	if first == "" {
		issue(result, 1, "import contains no nodes")
		return
	}
	// Detect static client profiles before scanning for URI-looking text in
	// their ignored rule/script sections. The adapter never fetches providers
	// or imports global policy, and leaves other formats untouched.
	if tryParseClientText(content, result) {
		return
	}
	if isURILine(first) || looksLikeURIList(trimmed) {
		scanner := bufio.NewScanner(strings.NewReader(content))
		scanner.Buffer(make([]byte, 4096), MaxContentBytes+1)
		line, attempts := 0, 0
		statusSeen := false
		for scanner.Scan() {
			line++
			text := strings.TrimSpace(scanner.Text())
			if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, "//") {
				continue
			}
			// Some URI subscriptions prepend a STATUS= account-information line.
			// It is presentation only, never a node or a request. Accept at most
			// one bounded header before entries; unknown/late lines still fail.
			if strings.HasPrefix(text, "STATUS=") && attempts == 0 && !statusSeen {
				statusSeen = true
				value := strings.TrimPrefix(text, "STATUS=")
				if value == "" || len(value) > 1024 || containsControl(value) {
					issue(result, line, "invalid subscription status header")
				}
				continue
			}
			attempts++
			if attempts > MaxNodes {
				issue(result, line, "import exceeds the 5000 node limit")
				break
			}
			node, err := parseURI(text)
			if err != nil {
				issue(result, line, err.Error())
			} else {
				result.Nodes = append(result.Nodes, node)
			}
		}
		if scanner.Err() != nil {
			issue(result, line, "URI line exceeds the input limit")
		}
		return
	}
	if decoded, ok := decodeBase64(trimmed, true); ok {
		if depth >= maxBase64Depth {
			issue(result, 1, "base64 nesting exceeds the supported limit")
			return
		}
		parseContent(decoded, depth+1, result)
		return
	}
	parseDocument(content, result)
}

func firstDataLine(s string) string {
	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Buffer(make([]byte, 4096), MaxContentBytes+1)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "//") {
			return line
		}
	}
	return ""
}

func looksLikeURIList(content string) bool {
	first := firstDataLine(content)
	if !strings.HasPrefix(first, "STATUS=") && (strings.HasPrefix(first, "{") || strings.HasPrefix(first, "[") || strings.HasPrefix(first, "-") || strings.Contains(first, ": ") || strings.HasSuffix(first, ":")) {
		return false
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 4096), MaxContentBytes+1)
	for scanner.Scan() {
		line := scanner.Text()
		if isURILine(strings.TrimSpace(line)) {
			// A STATUS-looking YAML key can introduce a block scalar. Its
			// indented contents are data, not independently importable nodes.
			// STATUS URI subscriptions require unindented entry lines so the
			// metadata exception cannot steal a genuine YAML document.
			if strings.HasPrefix(first, "STATUS=") && line != strings.TrimLeft(line, " \t") {
				return false
			}
			return true
		}
	}
	return false
}

func isURILine(s string) bool {
	i := strings.Index(s, "://")
	if i < 1 || i > 24 {
		return false
	}
	for _, c := range s[:i] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func decodeBase64(s string, whitespace bool) (string, bool) {
	if whitespace {
		s = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, s)
	}
	if s == "" || len(s) > MaxContentBytes {
		return "", false
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding.Strict(), base64.RawStdEncoding.Strict(), base64.URLEncoding.Strict(), base64.RawURLEncoding.Strict()} {
		b, err := encoding.DecodeString(s)
		if err == nil && len(b) <= MaxContentBytes && utf8.Valid(b) {
			return string(b), true
		}
	}
	return "", false
}

// Flow-style YAML may begin with '{' or '[' too. A quoted initial mapping
// key identifies JSON-style input, which receives strict JSON syntax checks.
func looksJSON(content string) bool {
	s := strings.TrimSpace(content)
	if strings.HasPrefix(s, "[") {
		s = strings.TrimSpace(s[1:])
	}
	if !strings.HasPrefix(s, "{") {
		return false
	}
	s = strings.TrimSpace(s[1:])
	return strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "}")
}

func parseDocument(content string, result *model.ParseResult) {
	trimmed := strings.TrimSpace(content)
	if looksJSON(trimmed) && !json.Valid([]byte(trimmed)) {
		issue(result, 1, "invalid JSON document")
		return
	}
	root, err := documentNode(content)
	if err != nil {
		issue(result, 1, err.Error())
		return
	}
	var list *yaml.Node
	singbox := false
	switch root.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(root.Content); i += 2 {
			key := root.Content[i].Value
			if unsafeWrapperKey(key) {
				issue(result, root.Content[i].Line, "external providers, scripts, and file references are not allowed")
				return
			}
			if key == "proxies" || key == "outbounds" {
				if list != nil {
					issue(result, root.Content[i].Line, "ambiguous document: both proxies and outbounds are present")
					return
				}
				list, singbox = root.Content[i+1], key == "outbounds"
			}
		}
		if list == nil {
			// A standalone outbound is also useful for exported snippets.
			if hasYAMLKey(root, "type") {
				list = &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{root}}
				singbox = hasYAMLKey(root, "server_port")
			} else {
				issue(result, root.Line, "expected a proxies or outbounds array")
				return
			}
		}
	case yaml.SequenceNode:
		list = root
	default:
		issue(result, root.Line, "expected URI links, base64, or a proxy configuration document")
		return
	}
	if list.Kind != yaml.SequenceNode {
		issue(result, list.Line, "proxies or outbounds must be an array")
		return
	}
	if len(list.Content) > MaxNodes {
		issue(result, list.Line, "import exceeds the 5000 node limit")
		return
	}
	if !singbox {
		parseClashNodes(root, list, result)
		return
	}
	for _, entry := range list.Content {
		value, err := nodeValue(entry)
		if err != nil {
			issue(result, entry.Line, err.Error())
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			issue(result, entry.Line, "outbound entry must be an object")
			continue
		}
		obj, err = fromSingbox(obj)
		if err == nil {
			var node model.ParsedNode
			node, err = normalizeNative(obj)
			if err == nil {
				result.Nodes = append(result.Nodes, node)
			}
		}
		if err != nil {
			issue(result, entry.Line, err.Error())
		}
	}
}

func hasYAMLKey(n *yaml.Node, key string) bool {
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return true
		}
	}
	return false
}

func unsafeWrapperKey(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(key, "_", "-")) {
	case "script", "scripts", "proxy-providers", "rule-providers", "providers", "external-ui", "external-ui-url", "external-controller-unix", "certificate-path", "key-path", "private-key-path", "ca-path", "rule-set":
		return true
	}
	return false
}

func documentNode(content string) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(strings.NewReader(content))
	var doc, extra yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, errors.New("invalid YAML/JSON document")
	}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("multiple documents or trailing data are not supported")
	}
	count := 0
	if err := checkYAML(&doc, 0, &count); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, errors.New("empty configuration document")
	}
	return doc.Content[0], nil
}

func checkYAML(n *yaml.Node, depth int, count *int) error {
	*count++
	if depth > maxDepth || *count > maxDocumentValues {
		return errors.New("configuration nesting or value count exceeds the supported limit")
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return errors.New("YAML anchors, aliases, and merge keys are not supported")
	}
	if n.Kind == yaml.MappingNode {
		keys := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" {
				return errors.New("configuration object keys must be plain strings")
			}
			if keys[key.Value] {
				return errors.New("duplicate configuration key")
			}
			keys[key.Value] = true
		}
	}
	switch n.Tag {
	case "", "!!map", "!!seq", "!!str", "!!int", "!!float", "!!bool", "!!null":
	default:
		return errors.New("unsupported YAML scalar tag")
	}
	for _, child := range n.Content {
		if err := checkYAML(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func nodeValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.MappingNode:
		obj := make(map[string]any, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			v, err := nodeValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			obj[n.Content[i].Value] = v
		}
		return obj, nil
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, child := range n.Content {
			v, err := nodeValue(child)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case yaml.ScalarNode:
		var value any
		if err := n.Decode(&value); err != nil {
			return nil, errors.New("invalid configuration scalar")
		}
		switch value.(type) {
		case nil, string, bool, int, int64, uint64, float64:
			return value, nil
		}
	}
	return nil, errors.New("unsupported configuration value")
}

// Fingerprint is a stable SHA-256 identity including credentials, connection
// options, and the resolved upstream identity, but not display/reference names.
// Direct-node hashes remain compatible with imports predating relay support.
// Invalid caller-supplied maps or unresolved dependencies return an empty string.
func Fingerprint(node model.ParsedNode) string {
	if _, unsafe := node.Config["dialer-proxy"]; unsafe || node.DialerProxy != "" && node.DialerFingerprint == "" {
		return ""
	}
	if node.DialerFingerprint != "" {
		decoded, err := hex.DecodeString(node.DialerFingerprint)
		if err != nil || len(decoded) != sha256.Size {
			return ""
		}
	}
	config := make(map[string]any, len(node.Config))
	for k, v := range node.Config {
		if k != "name" {
			config[k] = v
		}
	}
	identity := struct {
		Protocol          string         `json:"protocol"`
		Server            string         `json:"server"`
		Port              int            `json:"port"`
		UDP               bool           `json:"udp"`
		Config            map[string]any `json:"config"`
		DialerFingerprint string         `json:"dialerFingerprint,omitempty"`
	}{node.Protocol, node.Server, node.Port, node.UDP, config, node.DialerFingerprint}
	data, err := json.Marshal(identity)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func defaultName(protocol, server string, port int) string {
	return protocol + "://" + net.JoinHostPort(server, strconv.Itoa(port))
}

// truncateName shortens a display label on a rune boundary so preview and the
// management API agree on what can be stored. Control characters are rejected
// by the caller; identity never derives from this label.
func truncateName(value string, limit int) string {
	if limit < 1 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}
