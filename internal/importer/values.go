package importer

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math"
	"math/big"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// fields is a consuming reader: anything not deliberately supported is an
// error. In particular a future core feature cannot accidentally add file I/O
// or delegation to an imported outbound. Error messages never contain values.
type fields struct {
	values map[string]any
	used   map[string]bool
	err    error
}

func newFields(v map[string]any) *fields { return &fields{values: v, used: map[string]bool{}} }
func (r *fields) fail(message string) {
	if r.err == nil {
		r.err = errors.New(message)
	}
}
func (r *fields) has(key string) bool { _, ok := r.values[key]; return ok }
func (r *fields) get(key string) (any, bool) {
	r.used[key] = true
	v, ok := r.values[key]
	return v, ok
}
func (r *fields) text(key string, required bool) string {
	v, ok := r.get(key)
	if !ok {
		if required {
			r.fail("missing required " + key)
		}
		return ""
	}
	s, ok := v.(string)
	if !ok || !utf8.ValidString(s) || strings.ContainsRune(s, '\x00') || len(s) > 65536 {
		r.fail("invalid " + key)
		return ""
	}
	if required && s == "" {
		r.fail("missing required " + key)
	}
	return s
}
func (r *fields) boolean(key string, fallback bool) bool {
	v, ok := r.get(key)
	if !ok {
		return fallback
	}
	b, valid := boolValue(v)
	if !valid {
		r.fail("invalid boolean " + key)
		return fallback
	}
	return b
}
func boolValue(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch strings.ToLower(b) {
		case "1", "true":
			return true, true
		case "0", "false":
			return false, true
		}
	case int:
		if b == 0 || b == 1 {
			return b == 1, true
		}
	}
	return false, false
}
func (r *fields) integer(key string, fallback, low, high int) int {
	v, ok := r.get(key)
	if !ok {
		return fallback
	}
	n, valid := integerValue(v)
	if !valid || n < low || n > high {
		r.fail("invalid integer " + key)
		return fallback
	}
	return n
}
func integerValue(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		if int64(int(n)) == n {
			return int(n), true
		}
	case uint64:
		if n <= uint64(^uint(0)>>1) {
			return int(n), true
		}
	case float64:
		if !math.IsNaN(n) && !math.IsInf(n, 0) && n >= -2147483648 && n <= 2147483647 && math.Trunc(n) == n {
			return int(n), true
		}
	case string:
		if n == "" {
			return 0, false
		}
		for i, c := range n {
			if (c < '0' || c > '9') && !(i == 0 && c == '-') {
				return 0, false
			}
		}
		result, err := strconv.Atoi(n)
		return result, err == nil
	}
	return 0, false
}
func (r *fields) object(key string) map[string]any {
	v, ok := r.get(key)
	if !ok {
		return nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		r.fail("invalid object " + key)
		return nil
	}
	return obj
}
func (r *fields) strings(key string) []string {
	v, ok := r.get(key)
	if !ok {
		return nil
	}
	result := []string{}
	switch list := v.(type) {
	case []string:
		result = append([]string{}, list...)
	case []any:
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				r.fail("invalid string list " + key)
				return nil
			}
			result = append(result, s)
		}
	default:
		r.fail("invalid string list " + key)
		return nil
	}
	if len(result) == 0 {
		r.fail("empty string list " + key)
	}
	if len(result) > 256 {
		r.fail("too many entries in " + key)
	}
	for _, s := range result {
		if s == "" || !utf8.ValidString(s) || len(s) > 8192 || containsControl(s) {
			r.fail("invalid string list " + key)
		}
	}
	return result
}
func (r *fields) finish() error {
	unknown, unsafe := false, false
	for key := range r.values {
		if !r.used[key] {
			unknown = true
			unsafe = unsafe || unsafeOptionKey(key)
		}
	}
	if unsafe {
		r.fail("unsafe file, delegation, or executable option is not allowed")
	} else if unknown {
		r.fail("unsupported outbound option")
	}
	return r.err
}
func unsafeOptionKey(key string) bool {
	k := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
	if strings.HasSuffix(k, "-path") || strings.HasSuffix(k, "-file") {
		return true
	}
	switch k {
	case "dialer-proxy", "detour", "script", "scripts", "exec", "command", "plugin-path", "ca", "ca-str", "certificate", "private-key", "key", "cert", "interface-name", "bind-interface", "routing-mark", "sockopt", "realm-opts", "provider", "providers":
		return true
	}
	return false
}
func containsControl(s string) bool {
	for _, c := range s {
		if unicode.IsControl(c) {
			return true
		}
	}
	return false
}
func oneOf(s string, options ...string) bool {
	for _, option := range options {
		if s == option {
			return true
		}
	}
	return false
}
func copyText(r *fields, out map[string]any, key string) {
	if r.has(key) {
		out[key] = r.text(key, false)
	}
}
func copyBool(r *fields, out map[string]any, key string) {
	if r.has(key) {
		out[key] = r.boolean(key, false)
	}
}
func copyInt(r *fields, out map[string]any, key string, low, high int) {
	if r.has(key) {
		out[key] = r.integer(key, low, low, high)
	}
}
func canonicalProtocol(s string) string {
	switch strings.ToLower(s) {
	case "vless", "vmess", "trojan", "ss", "hysteria", "hysteria2", "tuic", "ssh", "socks5", "http":
		return strings.ToLower(s)
	case "shadowsocks":
		return "ss"
	case "hysteria1", "hy", "hy1":
		return "hysteria"
	case "hy2":
		return "hysteria2"
	case "socks", "socks5h", "socks5s":
		return "socks5"
	case "https":
		return "http"
	}
	return ""
}
func validHost(s string) (string, bool) {
	if len(s) > 253 || s == "" || strings.TrimSpace(s) != s || containsControl(s) || strings.ContainsAny(s, "/@\\?#%") {
		return "", false
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = s[1 : len(s)-1]
	}
	if addr, err := netip.ParseAddr(s); err == nil {
		if addr.Zone() != "" {
			return "", false
		}
		return addr.String(), true
	}
	s = strings.TrimSuffix(s, ".")
	if s == "" {
		return "", false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return "", false
			}
		}
	}
	return strings.ToLower(s), true
}

var uuidPattern = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

func normalizeUUID(r *fields, key string) string {
	s := r.text(key, true)
	if !uuidPattern.MatchString(s) {
		r.fail("invalid UUID credential")
	}
	return strings.ToLower(s)
}
func validPublicKey(s string) bool {
	b, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimRight(s, "="))
	return err == nil && len(b) == 32
}
func validShortID(s string) bool {
	if len(s) > 16 || len(s)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func normalizePin(s string) (string, bool) {
	s = strings.ReplaceAll(s, ":", "")
	if len(s) != 64 {
		return "", false
	}
	_, err := hex.DecodeString(s)
	return strings.ToLower(s), err == nil
}

var bandwidthPattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(?:[ \t]*([KMGTkmgt]?)([Bb])(?:ps|/s))?$`)

func bandwidth(r *fields, key string, required bool) any {
	v, ok := r.get(key)
	if !ok {
		if required {
			r.fail("missing required " + key)
		}
		return nil
	}
	s, valid := v.(string)
	if !valid {
		if n, ok := integerValue(v); ok {
			s, valid = strconv.Itoa(n), true
		}
	}
	if !valid || len(s) > 128 {
		r.fail("invalid bandwidth " + key)
		return nil
	}
	match := bandwidthPattern.FindStringSubmatch(s)
	if match == nil {
		r.fail("invalid bandwidth " + key)
		return nil
	}
	rate, ok := new(big.Rat).SetString(match[1])
	if !ok || rate.Sign() <= 0 {
		r.fail("invalid bandwidth " + key)
		return nil
	}
	unit, bit := strings.ToUpper(match[2]), match[3]
	if bit == "" {
		unit, bit = "M", "b"
	}
	scale := int64(1)
	switch unit {
	case "K":
		scale = 1000
	case "M":
		scale = 1000000
	case "G":
		scale = 1000000000
	case "T":
		scale = 1000000000000
	}
	rate.Mul(rate, new(big.Rat).SetInt64(scale))
	if bit == "b" {
		rate.Quo(rate, new(big.Rat).SetInt64(8))
	}
	// Mihomo silently treats an invalid rate as unlimited in Hysteria2. Emit
	// canonical integral bytes/second and reject overflow or fractional bytes.
	if !rate.IsInt() || !rate.Num().IsInt64() || rate.Num().Int64() < 1 || rate.Num().Int64() > 1000000000000 {
		r.fail("bandwidth is out of range or not representable")
		return nil
	}
	return rate.Num().String() + " Bps"
}
func validatePortRanges(s string) bool {
	if s == "" || len(s) > 4096 {
		return false
	}
	parts := strings.Split(s, ",")
	if len(parts) > 128 {
		return false
	}
	for _, part := range parts {
		pair := strings.Split(part, "-")
		if len(pair) > 2 {
			return false
		}
		first, e := strconv.Atoi(pair[0])
		if e != nil || first < 1 || first > 65535 {
			return false
		}
		if len(pair) == 2 {
			last, e := strconv.Atoi(pair[1])
			if e != nil || last < first || last > 65535 {
				return false
			}
		}
	}
	return true
}
func inlinePrivateKey(s string) bool {
	// Mihomo treats a non-PEM string as a filesystem path. Reject it even if the
	// file does not currently exist; imports must never gain host file access.
	if !strings.HasPrefix(strings.TrimSpace(s), "-----BEGIN ") {
		return false
	}
	block, rest := pem.Decode([]byte(s))
	return block != nil && strings.TrimSpace(string(rest)) == "" && strings.HasSuffix(block.Type, "PRIVATE KEY") && len(block.Bytes) > 0
}
func headerName(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_\u0060|~", c)) {
			return false
		}
	}
	return true
}
func cleanHeaders(obj map[string]any, listValues bool) (map[string]any, error) {
	if len(obj) > 128 {
		return nil, errors.New("too many transport headers")
	}
	out := map[string]any{}
	seen := map[string]bool{}
	for key, v := range obj {
		if !headerName(key) || seen[strings.ToLower(key)] {
			return nil, errors.New("invalid or duplicate transport header")
		}
		seen[strings.ToLower(key)] = true
		if key == "host" || strings.EqualFold(key, "host") {
			key = "Host"
		}
		if listValues {
			r := newFields(map[string]any{"headers": v})
			list := r.strings("headers")
			if r.err != nil {
				return nil, errors.New("invalid transport header values")
			}
			out[key] = list
		} else {
			s, ok := v.(string)
			if !ok || !utf8.ValidString(s) || containsControl(s) || len(s) > 8192 {
				return nil, errors.New("invalid transport header value")
			}
			out[key] = s
		}
	}
	return out, nil
}
