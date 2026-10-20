package importer

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"unicode/utf8"

	"github.com/uniproxy/uniproxy/internal/model"
)

const (
	maxClientLineBytes  = 256 * 1024
	maxClientFieldBytes = 65536
	maxClientFields     = 128
)

// tryParseClientText recognizes static Quantumult X and Surge/Surfboard/Loon
// assignments. Full profiles are containers, not executable configurations: only
// their local node sections are read. All output still passes normalizeNative.
// A false return leaves result untouched so URI, base64 and YAML/JSON routing can
// proceed. Detection deliberately does not search inside a structured document.
func tryParseClientText(content string, result *model.ParseResult) bool {
	contentBytes := len(content)
	content = strings.TrimPrefix(content, "\ufeff")
	probe := content
	if len(probe) > MaxContentBytes {
		probe = probe[:MaxContentBytes]
	}
	recognized, document := recognizeClientText(probe)
	if !recognized {
		return false
	}
	if contentBytes > MaxContentBytes {
		issue(result, 1, "import exceeds the 5 MiB limit")
		return true
	}
	if !utf8.ValidString(content) {
		issue(result, 1, "import must contain valid UTF-8 text")
		return true
	}

	initialNodes, initialErrors := len(result.Nodes), result.TotalErrors
	section := ""
	managedHeader := false
	scanner := clientScanner(content)
	line, attempts := 0, 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if document {
			if name, ok := clientSection(text); ok {
				section = name
				continue
			}
			if strings.HasPrefix(text, "[") && !clientAssignmentShape(text) {
				issue(result, line, "malformed client configuration section")
				section = "invalid"
				continue
			}
		}
		if document && section == "" && strings.HasPrefix(text, "#!MANAGED-CONFIG") {
			// This root-only marker advertises how the source client refreshes
			// its profile. It supplies no extra nodes; keep no URL and do no I/O.
			payload, marker := strings.CutPrefix(text, "#!MANAGED-CONFIG ")
			if managedHeader || !marker || strings.TrimSpace(payload) == "" || len(text) > 8192 || containsControl(text) {
				issue(result, line, "invalid or duplicate managed client profile header")
			}
			managedHeader = true
			continue
		}
		nodeSection := section == "proxy" || section == "server_local"
		remoteSection := section == "server_remote" || section == "remote proxy"
		groupSection := section == "proxy group" || section == "policy"
		// Surge's include/requirement directives look like comments but can
		// supply or conditionally omit nodes. Never treat them as inert comments.
		directive := strings.HasPrefix(text, "#!") && (!document || section == "" || nodeSection || remoteSection || groupSection)
		if clientComment(text) && !directive {
			continue
		}
		var groupError error
		if groupSection && !directive {
			groupError = inspectClientGroup(text)
			if groupError == nil {
				continue
			}
		}
		if document && !nodeSection && !remoteSection && !groupSection && section != "" {
			continue
		}
		if document && section == "proxy" && canonicalClientBuiltin(text) {
			// These exact declarations are document policy, not proxy nodes.
			// Aliases, parameters, raw-list builtins and other pseudo-nodes fail.
			continue
		}
		attempts++
		if attempts > MaxNodes {
			issue(result, line, "import exceeds the 5000 node limit")
			break
		}
		if directive {
			issue(result, line, "client node include or conditional directives are unsupported")
			continue
		}
		if groupError != nil {
			issue(result, line, groupError.Error())
			continue
		}
		if remoteSection {
			issue(result, line, "remote proxy providers cannot be imported offline")
			continue
		}
		if document && section == "" {
			issue(result, line, "expected a client configuration section")
			continue
		}
		node, err := parseClientAssignment(text, section)
		if err != nil {
			issue(result, line, err.Error())
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	if scanner.Err() != nil {
		issue(result, line, "client configuration line exceeds the input limit")
	}
	if len(result.Nodes) == initialNodes && result.TotalErrors == initialErrors {
		issue(result, 1, "client configuration contains no static proxy nodes")
	}
	return true
}

func clientScanner(content string) *bufio.Scanner {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 4096), MaxContentBytes+1)
	return scanner
}

func clientComment(text string) bool {
	return strings.HasPrefix(text, "#") || strings.HasPrefix(text, ";") || strings.HasPrefix(text, "//")
}

func clientSection(text string) (string, bool) {
	if len(text) < 3 || len(text) > 128 || text[0] != '[' || text[len(text)-1] != ']' {
		return "", false
	}
	name := strings.TrimSpace(text[1 : len(text)-1])
	if name == "" {
		return "", false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == ' ') {
			return "", false
		}
	}
	return strings.ToLower(strings.Join(strings.Fields(name), " ")), true
}

func recognizeClientText(content string) (recognized, document bool) {
	scanner := clientScanner(content)
	first, rawURI := true, false
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || clientComment(text) {
			continue
		}
		section, isSection := clientSection(text)
		if first {
			first = false
			if isURILine(text) || clientStructuredStart(text, isSection) {
				return false, false
			}
		}
		if isSection {
			if oneOf(section, "proxy", "server_local", "server_remote", "remote proxy", "proxy group", "policy") {
				return true, true
			}
			// Remember the container even when its first section is General,
			// DNS, Script, etc. Its contents must never trigger raw-list mode.
			document = true
			continue
		}
		if !document {
			rawURI = rawURI || isURILine(text)
			if clientAssignmentShape(text) {
				recognized = true
			}
		}
	}
	// A profile with no local/remote node section is not a client node source.
	// Mixed raw URI lists (including a STATUS= header) belong to the URI layer.
	return recognized && !document && !rawURI, false
}

func clientStructuredStart(text string, section bool) bool {
	// YAML explicit keys can precede a value several lines later; do not scan
	// their nested scalar labels for an accidental client assignment.
	if text == "?" || strings.HasPrefix(text, "? ") || strings.HasPrefix(text, "?\t") {
		return true
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "-") || strings.HasPrefix(text, "'") || strings.HasPrefix(text, "%") || strings.HasPrefix(text, "|") || strings.HasPrefix(text, ">") || strings.HasPrefix(text, "&") || strings.HasPrefix(text, "!") || strings.HasPrefix(text, "<") {
		return true
	}
	if (strings.HasPrefix(text, "[") && !section || strings.HasPrefix(text, "\"")) && !clientAssignmentShape(text) {
		return true
	}
	// A YAML mapping (including a block scalar containing assignments) must be
	// left to the document parser. Colons inside a quoted display label are OK.
	quoted := false
	for i := 0; i < len(text); i++ {
		if quoted && text[i] == '\\' {
			i++
			continue
		}
		if text[i] == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if text[i] == ':' && (i+1 == len(text) || text[i+1] == ' ' || text[i+1] == '\t') {
			return true
		}
	}
	return false
}

func clientAssignmentShape(text string) bool {
	if isURILine(text) {
		return false
	}
	left, right, ok := clientPair(text)
	if !ok {
		return false
	}
	if clientProtocol(strings.TrimSpace(left)) != "" {
		return true
	}
	// Only the first RHS CSV field is a protocol. In particular an equals sign
	// in URI query data or a quoted positional secret is not a discriminator.
	if i := strings.IndexByte(right, ','); i >= 0 {
		right = right[:i]
	}
	return clientProtocol(strings.TrimSpace(right)) != ""
}

func clientProtocol(value string) string {
	switch strings.ToLower(value) {
	case "trojan", "vmess", "vless", "http", "https", "ss", "shadowsocks", "socks5":
		return canonicalProtocol(value)
	case "socks5-tls":
		return "socks5"
	}
	return ""
}

// Groups themselves are never imported. However Surge policy-path supplies
// additional proxy definitions, so ignoring it would falsely mark a partial
// subscription as complete. Inspect syntax only; do not read files, fetch URLs,
// expand other groups, evaluate regular expressions, or run policy modifiers.
func inspectClientGroup(text string) error {
	parts, err := splitClientCSV(text, MaxNodes+maxClientFields)
	if err != nil {
		return errors.New("client policy group cannot be safely inspected")
	}
	for index, part := range parts {
		key, value, pair := clientPair(part)
		if !pair {
			if _, err := clientScalar(part); err != nil {
				return errors.New("malformed client policy group field")
			}
			continue
		}
		key, err = clientScalar(key)
		if err != nil {
			return errors.New("malformed client policy group field")
		}
		key = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "_", "-")
		// The first assignment declares the group name/type, not an option;
		// a group may legitimately be named policy-path. Only later fields
		// can request externally supplied nodes.
		if index > 0 && key == "policy-path" {
			return errors.New("external client policy groups cannot be imported offline")
		}
		if _, err := clientScalar(value); err != nil {
			return errors.New("malformed client policy group field")
		}
	}
	return nil
}

func canonicalClientBuiltin(text string) bool {
	left, right, ok := strings.Cut(text, "=")
	if !ok {
		return false
	}
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	return (left == "DIRECT" || left == "REJECT") && strings.EqualFold(left, right)
}

// clientCSV retains quoting until each scalar or key/value pair is interpreted.
// Commas and equals signs in a quoted secret therefore remain data. The only
// escapes are the documented double quote and backslash; multiline values and
// ambiguous inline comments are rejected, never truncated into new credentials.
func clientCSV(text string) ([]string, error) {
	return splitClientCSV(text, maxClientFields)
}

func splitClientCSV(text string, fieldLimit int) ([]string, error) {
	if len(text) > maxClientLineBytes {
		return nil, errors.New("client proxy line exceeds the supported limit")
	}
	parts := make([]string, 0, 12)
	quoted, start := false, 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quoted && c == '\\' {
			if i+1 >= len(text) || text[i+1] != '\\' && text[i+1] != '"' {
				return nil, errors.New("invalid quoted client value escape")
			}
			i++
			continue
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && i > 0 && (text[i-1] == ' ' || text[i-1] == '\t') && (c == '#' || c == ';' || c == '/' && i+1 < len(text) && text[i+1] == '/') {
			return nil, errors.New("inline client comments are ambiguous; use whole-line comments")
		}
		if c == ',' && !quoted {
			parts = append(parts, text[start:i])
			start = i + 1
			if len(parts) >= fieldLimit {
				return nil, errors.New("too many client proxy fields")
			}
		}
	}
	if quoted {
		return nil, errors.New("unterminated quoted client value")
	}
	parts = append(parts, text[start:])
	return parts, nil
}

func clientPair(text string) (left, right string, ok bool) {
	quoted := false
	for i := 0; i < len(text); i++ {
		if quoted && text[i] == '\\' {
			i++
			continue
		}
		if text[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted {
			if text[i] == ',' {
				return "", "", false
			}
			if text[i] == '=' {
				return text[:i], text[i+1:], true
			}
		}
	}
	return "", "", false
}

func clientScalar(text string) (string, error) {
	text = strings.TrimSpace(text)
	if len(text) > maxClientFieldBytes || !utf8.ValidString(text) || containsControl(text) {
		return "", errors.New("invalid or oversized client value")
	}
	if !strings.HasPrefix(text, "\"") {
		if strings.ContainsRune(text, '"') {
			return "", errors.New("malformed quoted client value")
		}
		return text, nil
	}
	var value strings.Builder
	for i := 1; i < len(text); i++ {
		c := text[i]
		if c == '"' {
			if i != len(text)-1 {
				return "", errors.New("unexpected data after quoted client value")
			}
			return value.String(), nil
		}
		if c == '\\' {
			i++
			if i >= len(text) || text[i] != '"' && text[i] != '\\' {
				return "", errors.New("invalid quoted client value escape")
			}
			c = text[i]
		}
		value.WriteByte(c)
	}
	return "", errors.New("unterminated quoted client value")
}

type clientOptions struct {
	values map[string]string
	used   map[string]bool
	err    error
}

func (o *clientOptions) fail(message string) {
	if o.err == nil {
		o.err = errors.New(message)
	}
}

func (o *clientOptions) take(keys ...string) (value string, present bool) {
	for _, key := range keys {
		o.used[key] = true
		if v, ok := o.values[key]; ok {
			if present {
				o.fail("conflicting client option aliases")
			}
			value, present = v, true
		}
	}
	return
}

func (o *clientOptions) set(out map[string]any, key string, aliases ...string) {
	if value, ok := o.take(aliases...); ok {
		if _, exists := out[key]; exists {
			o.fail("duplicate or conflicting client field")
		}
		out[key] = value
	}
}

func (o *clientOptions) flag(out map[string]any, key string, aliases ...string) {
	if value, ok := o.take(aliases...); ok {
		flag, valid := boolValue(value)
		if !valid {
			o.fail("invalid client boolean option")
		}
		if _, exists := out[key]; exists {
			o.fail("duplicate or conflicting client field")
		}
		out[key] = flag
	}
}

func (o *clientOptions) finish() error {
	unknown, unsafe := false, false
	for key := range o.values {
		if !o.used[key] {
			unknown = true
			unsafe = unsafe || unsafeOptionKey(key) || oneOf(key, "underlying-proxy", "interface", "via", "include", "parser", "parser-plugin", "plugin", "url", "path-file")
		}
	}
	if unsafe {
		o.fail("unsafe client file, provider, delegation or executable option")
	} else if unknown {
		o.fail("unsupported client proxy option")
	}
	return o.err
}

func readClientOptions(parts []string) (*clientOptions, []string, error) {
	o := &clientOptions{values: map[string]string{}, used: map[string]bool{}}
	var positional []string
	named := false
	for _, part := range parts {
		left, right, pair := clientPair(part)
		if !pair {
			value, err := clientScalar(part)
			if err != nil {
				return nil, nil, err
			}
			if named || value == "" && strings.TrimSpace(part) == "" {
				return nil, nil, errors.New("malformed positional client field")
			}
			positional = append(positional, value)
			continue
		}
		named = true
		key := strings.ToLower(strings.TrimSpace(left))
		if key == "" || len(key) > 64 {
			return nil, nil, errors.New("malformed client option name")
		}
		for _, c := range key {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return nil, nil, errors.New("malformed client option name")
			}
		}
		value, err := clientScalar(right)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := o.values[key]; exists {
			return nil, nil, errors.New("duplicate client option")
		}
		o.values[key] = value
	}
	return o, positional, nil
}

func parseClientAssignment(text, section string) (model.ParsedNode, error) {
	parts, err := clientCSV(text)
	if err != nil {
		return model.ParsedNode{}, err
	}
	left, right, pair := clientPair(parts[0])
	if !pair {
		return model.ParsedNode{}, errors.New("expected a static client proxy assignment")
	}
	name, err := clientScalar(left)
	if err != nil {
		return model.ParsedNode{}, err
	}
	first, err := clientScalar(right)
	if err != nil {
		return model.ParsedNode{}, err
	}
	// RHS protocol takes precedence: a named node may itself be called VMess.
	qx := clientProtocol(first) == "" && clientProtocol(name) != ""
	if section == "server_local" && !qx || section == "proxy" && qx {
		return model.ParsedNode{}, errors.New("proxy assignment does not match its client section")
	}
	rawType := first
	optionStart := 3
	out := map[string]any{"name": name}
	if qx {
		rawType = name
		host, port, err := net.SplitHostPort(first)
		if err != nil || host == "" || port == "" {
			return model.ParsedNode{}, errors.New("invalid client proxy endpoint; IPv6 requires brackets")
		}
		out["server"], out["port"] = host, port
		delete(out, "name")
		optionStart = 1
	} else {
		if clientProtocol(rawType) == "" {
			return model.ParsedNode{}, errors.New("unsupported client proxy protocol or pseudo-node")
		}
		if name == "" || len(parts) < 3 {
			return model.ParsedNode{}, errors.New("missing client proxy name, server or port")
		}
		for i, key := range []string{"server", "port"} {
			value, err := clientScalar(parts[i+1])
			if err != nil {
				return model.ParsedNode{}, err
			}
			out[key] = value
		}
	}
	protocol := clientProtocol(rawType)
	out["type"] = strings.ToLower(rawType)
	if strings.EqualFold(rawType, "socks5-tls") {
		out["type"] = "socks5s"
	}
	o, positional, err := readClientOptions(parts[optionStart:])
	if err != nil {
		return model.ParsedNode{}, err
	}
	if qx && len(positional) != 0 {
		return model.ParsedNode{}, errors.New("unexpected positional Quantumult X field")
	}
	loon := !qx && len(positional) > 0
	if qx {
		o.set(out, "name", "tag")
	}
	mapClientCredentials(o, out, protocol, positional, qx)
	// Surge and Surfboard share this syntax but disagree on implicit UDP.
	// Static extraction deliberately enables UDP only when explicitly requested;
	// it is not an assertion that all source-client defaults are equivalent.
	o.flag(out, "udp", "udp", "udp-relay")
	o.flag(out, "tfo", "tfo", "fast-open")
	mapClientTLS(o, out)
	mapClientTransport(o, out, protocol, qx, loon)
	materializeClientWSIdentity(o, out, protocol)
	if !qx {
		// Surge's no-error-alert only changes its own presentation. It cannot
		// suppress UniProxy errors, become native config, or trigger a request.
		if v, ok := o.take("no-error-alert"); ok {
			if _, valid := boolValue(v); !valid {
				o.fail("invalid client presentation boolean")
			}
		}
	}
	if err := o.finish(); err != nil {
		return model.ParsedNode{}, err
	}
	return normalizeNative(out)
}

func mapClientCredentials(o *clientOptions, out map[string]any, protocol string, positional []string, qx bool) {
	switch protocol {
	case "ss", "vmess":
		if len(positional) != 0 && len(positional) != 2 {
			o.fail("expected client cipher and credential positional fields")
		}
		credential := "password"
		if protocol == "vmess" {
			credential = "uuid"
		}
		if len(positional) == 2 {
			out["cipher"], out[credential] = positional[0], positional[1]
		}
		o.set(out, "cipher", "method", "encrypt-method", "cipher")
		if protocol == "ss" {
			o.set(out, "password", "password")
		} else {
			o.set(out, "uuid", "password", "username", "uuid")
			if out["cipher"] == "chacha20-ietf-poly1305" {
				out["cipher"] = "chacha20-poly1305"
			}
			if _, ok := out["cipher"]; !ok {
				if qx {
					o.fail("VMess client cipher must be explicit")
				} else {
					out["cipher"] = "aes-128-gcm"
				}
			}
			o.set(out, "alterId", "alterid", "alter-id")
			if aead, ok := o.take("aead", "vmess-aead"); ok {
				enabled, valid := boolValue(aead)
				if !valid {
					o.fail("invalid VMess AEAD boolean")
				} else if !enabled {
					o.fail("legacy client VMess AEAD disable option is unsupported")
				} else if value, exists := out["alterId"]; exists {
					if n, valid := integerValue(value); !valid || n != 0 {
						o.fail("VMess AEAD conflicts with the configured alter ID")
					}
				}
			} else if !qx && len(positional) == 0 && out["alterId"] == nil {
				// Surge defaults to its legacy handshake, unlike QX and Loon's
				// explicit positional/alterId format. Never invent AEAD for it.
				o.fail("VMess client handshake must specify AEAD or an alter ID")
			}
		}
	case "trojan", "vless":
		if len(positional) > 1 {
			o.fail("unexpected client credential positional fields")
		}
		credential := "password"
		if protocol == "vless" {
			credential = "uuid"
		}
		if len(positional) == 1 {
			out[credential] = positional[0]
		}
		if protocol == "vless" {
			o.set(out, "uuid", "password", "username", "uuid")
			o.set(out, "encryption", "method", "encryption")
			o.set(out, "flow", "flow", "vless-flow")
		} else {
			o.set(out, "password", "password")
		}
	case "socks5", "http":
		if len(positional) != 0 && len(positional) != 2 {
			o.fail("expected client username and password positional fields")
		}
		if len(positional) == 2 {
			out["username"], out["password"] = positional[0], positional[1]
		}
		o.set(out, "username", "username")
		o.set(out, "password", "password")
	}
}

func mapClientTLS(o *clientOptions, out map[string]any) {
	o.flag(out, "tls", "over-tls", "tls")
	serverKey := "sni"
	if out["type"] == "vmess" || out["type"] == "vless" {
		serverKey = "servername"
	}
	if value, ok := o.take("sni", "tls-name", "tls-host"); ok {
		if strings.EqualFold(value, "off") {
			o.fail("disabling TLS SNI is unsupported")
		}
		out[serverKey] = value
	}
	skip, hasSkip := o.take("skip-cert-verify")
	verify, hasVerify := o.take("tls-verification")
	if hasSkip && hasVerify {
		o.fail("conflicting client certificate verification aliases")
	}
	if hasSkip || hasVerify {
		value := skip
		if hasVerify {
			value = verify
		}
		flag, valid := boolValue(value)
		if !valid {
			o.fail("invalid client certificate verification boolean")
		}
		if hasVerify {
			flag = !flag
		}
		out["skip-cert-verify"] = flag
	}
	o.set(out, "fingerprint", "tls-cert-sha256", "server-cert-fingerprint-sha256")
	o.set(out, "name-cert-verify", "server-cert-verify-name")
	if out["fingerprint"] != nil && out["skip-cert-verify"] == true {
		o.fail("client certificate pin conflicts with disabled verification")
	}
	if name, present := out["name-cert-verify"]; present && name != "" && out["skip-cert-verify"] == true {
		o.fail("client certificate verification name conflicts with disabled verification")
	}
	if value, ok := o.take("alpn"); ok {
		out["alpn"] = strings.Split(value, ",")
	}
}

func mapClientTransport(o *clientOptions, out map[string]any, protocol string, qx, loon bool) {
	mode, hasObfs := o.take("obfs", "obfs-name")
	transport, hasTransport := o.take("transport")
	ws, hasWS := o.take("ws")
	if hasWS {
		flag, valid := boolValue(ws)
		if !valid {
			o.fail("invalid client WebSocket boolean")
		}
		if hasTransport {
			o.fail("conflicting client transport aliases")
		}
		transport = "tcp"
		if flag {
			transport = "ws"
		}
		hasTransport = true
	}
	if hasObfs && hasTransport || qx && hasTransport {
		o.fail("conflicting or unsupported client transport fields")
	}
	if protocol == "ss" {
		mapClientShadowsocksTransport(o, out, mode, hasObfs, hasTransport, qx)
		return
	}
	if oneOf(protocol, "socks5", "http") {
		if hasObfs || hasTransport {
			o.fail("transport overrides are unsupported for this client protocol")
		}
		return
	}
	if transport == "" && !hasTransport {
		transport = "tcp"
	}
	if hasObfs {
		if !qx {
			o.fail("obfuscation is unsupported for this client protocol")
		}
		if _, explicitTLS := out["tls"]; explicitTLS {
			o.fail("client obfuscation conflicts with an explicit TLS option")
		}
		if qx && (out["sni"] != nil || out["servername"] != nil) {
			o.fail("client obfuscation TLS name must use its obfuscation host")
		}
		switch mode {
		case "none":
			transport = "tcp"
		case "over-tls":
			transport, out["tls"] = "tcp", true
		case "ws", "wss":
			transport, out["tls"] = "ws", mode == "wss"
		case "http":
			transport, out["tls"] = "http", false
		default:
			o.fail("unsupported client obfuscation mode")
		}
		if protocol == "trojan" && mode != "wss" {
			o.fail("Quantumult X Trojan obfuscation requires WebSocket over TLS")
		}
	}
	if loon && protocol == "trojan" && transport == "http" {
		// Loon explicitly defines this legacy Trojan spelling as WebSocket,
		// unlike its VMess/VLESS HTTP transport of the same name.
		transport = "ws"
	}
	if !oneOf(transport, "tcp", "ws", "http") {
		o.fail("unsupported client transport")
	}
	out["network"] = transport
	var host, path string
	var hasHost, hasPath bool
	if qx {
		host, hasHost = o.take("obfs-host")
		path, hasPath = o.take("obfs-uri")
	} else {
		host, hasHost = o.take("host", "ws-host")
		path, hasPath = o.take("path", "ws-path")
	}
	headers := map[string]any{}
	if value, ok := o.take("ws-headers"); ok {
		if qx || transport != "ws" {
			o.fail("WebSocket headers require a supported WebSocket transport")
		}
		parsed, err := clientWSHeaders(value)
		if err != nil {
			o.fail(err.Error())
		} else {
			headers = parsed
		}
	}
	if hasHost {
		if _, present := headers["Host"]; present {
			o.fail("conflicting client WebSocket host aliases")
		}
		headers["Host"] = host
	}
	if qx && hasObfs && (mode == "over-tls" || mode == "wss") && hasHost {
		key := "sni"
		if protocol == "vmess" || protocol == "vless" {
			key = "servername"
		}
		if _, present := out[key]; present {
			o.fail("conflicting client TLS host aliases")
		}
		out[key] = host
	}
	switch transport {
	case "ws":
		options := map[string]any{}
		if hasPath {
			options["path"] = path
		}
		if len(headers) > 0 {
			options["headers"] = headers
		}
		out["ws-opts"] = options
	case "http":
		options := map[string]any{}
		if hasPath {
			options["path"] = []string{path}
		}
		if hasHost {
			options["headers"] = map[string]any{"Host": []string{host}}
		}
		out["http-opts"] = options
	default:
		if hasPath || hasHost && !(qx && hasObfs && mode == "over-tls") {
			o.fail("client transport host or path would be ignored")
		}
	}
}

// Mihomo's VMess/VLESS WS path falls back to the HTTP Host for TLS identity.
// A client Host override is not evidence that the source also intended to
// authenticate that host. When a source default is ambiguous, require an
// explicit TLS name instead of silently changing the authenticated endpoint.
func materializeClientWSIdentity(o *clientOptions, out map[string]any, protocol string) {
	if out["network"] != "ws" || out["tls"] != true && protocol != "trojan" {
		return
	}
	key := "sni"
	if protocol == "vmess" || protocol == "vless" {
		key = "servername"
	}
	value, explicitName := out[key]
	explicitName = explicitName && value != ""
	server, _ := out["server"].(string)
	endpoint, valid := validHost(server)
	if !valid {
		// normalizeNative reports the invalid endpoint, with no raw value.
		return
	}
	options, _ := out["ws-opts"].(map[string]any)
	headers, _ := options["headers"].(map[string]any)
	if protocol == "trojan" {
		// The pinned Trojan WS path uses SNI as HTTP Host when no Host is
		// explicit. Do not let a TLS override silently change HTTP identity.
		if explicitName {
			name, _ := value.(string)
			normalized, valid := validHost(name)
			if (headers["Host"] == nil || headers["Host"] == "") && (!valid || normalized != endpoint) {
				o.fail("client Trojan WebSocket requires an explicit host when its TLS name differs")
			}
		} else {
			out[key] = endpoint
		}
		return
	}
	if explicitName {
		return
	}
	if host, present := headers["Host"]; present && host != "" {
		name, _ := host.(string)
		normalized, valid := validHost(name)
		if !valid || normalized != endpoint {
			o.fail("client WebSocket TLS requires an explicit server name when its host differs")
			return
		}
	}
	out[key] = endpoint
}

func clientWSHeaders(value string) (map[string]any, error) {
	if len(value) > maxClientFieldBytes {
		return nil, errors.New("client WebSocket headers exceed the supported limit")
	}
	headers := map[string]any{}
	seen := map[string]bool{}
	for _, entry := range strings.Split(value, "|") {
		key, val, ok := strings.Cut(entry, ":")
		key = strings.TrimSpace(key)
		lower := strings.ToLower(key)
		if !ok || !headerName(key) || seen[lower] || len(headers) >= 128 {
			return nil, errors.New("invalid or duplicate client WebSocket header")
		}
		seen[lower] = true
		if lower == "host" {
			key = "Host"
		}
		headers[key] = strings.TrimSpace(val)
	}
	return cleanHeaders(headers, false)
}

func mapClientShadowsocksTransport(o *clientOptions, out map[string]any, mode string, hasObfs, hasTransport, qx bool) {
	if hasTransport {
		o.fail("unsupported Shadowsocks client transport option")
	}
	host, hasHost := o.take("obfs-host")
	path, hasPath := o.take("obfs-uri")
	if !hasObfs || mode == "none" {
		if hasHost || hasPath {
			o.fail("Shadowsocks obfuscation options require an obfuscation mode")
		}
		return
	}
	if !hasHost || host == "" {
		// Mihomo has plugin-specific implicit hosts (including a third-party
		// hostname). Their equivalence to source-client defaults is unproven.
		o.fail("client Shadowsocks obfuscation requires an explicit host")
	}
	options := map[string]any{}
	switch mode {
	case "http", "tls":
		out["plugin"] = "obfs"
		options["mode"] = mode
		if hasHost {
			options["host"] = host
		}
		if hasPath {
			o.fail("Shadowsocks obfuscation paths are unsupported by the pinned core")
		}
	case "ws", "wss":
		if !qx {
			o.fail("unsupported Shadowsocks client obfuscation mode")
		}
		if _, present := out["tls"]; present {
			o.fail("client obfuscation conflicts with an explicit TLS option")
		}
		if out["sni"] != nil {
			o.fail("client plugin TLS name must use its obfuscation host")
		}
		out["plugin"] = "v2ray-plugin"
		options["mode"], options["tls"], options["mux"] = "websocket", mode == "wss", false
		options["host"] = host
		if hasPath {
			options["path"] = path
		}
		for _, key := range []string{"skip-cert-verify", "fingerprint", "name-cert-verify"} {
			if value, present := out[key]; present {
				options[key] = value
				delete(out, key)
			}
		}
	default:
		o.fail("unsupported Shadowsocks client obfuscation mode")
	}
	out["plugin-opts"] = options
}
