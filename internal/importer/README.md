# Offline importer

`Parse(content string) model.ParseResult` returns JSON-compatible native Mihomo outbound maps without a user-controlled `name` or `dialer-proxy`. `Fingerprint(model.ParsedNode)` returns a SHA-256 identity that excludes display/reference names but includes credentials, endpoint, UDP setting, TLS, transport options, and the recursively resolved upstream identity. Direct-node hashes remain bit-for-bit compatible with the pre-relay encoding. Invalid caller-created maps, raw config delegation, malformed dependency fingerprints, and unresolved references return an empty fingerprint.

## Accepted inputs

- One proxy URI per line, blank lines and `#`/`//` comments allowed. An invalid entry does not discard valid siblings.
- Standard and URL-safe base64 subscriptions, padded or unpadded, with whitespace wrapping and at most four nested encodings.
- Clash YAML/JSON `proxies`, standalone outbound objects, or outbound arrays. Non-node global configuration is never copied into a node. Native `dialer-proxy` references are supported only when the target is another explicitly named native proxy in that same document/array.
- sing-box JSON `outbounds` or standalone objects with `server_port`.
- Static Quantumult X `protocol=endpoint,...,tag=name` and Surge/Surfboard/Loon `name=protocol,...` assignments, bare or inside `[server_local]` / `[Proxy]`; supports Trojan, VMess, VLESS, Shadowsocks, HTTP(S), SOCKS5 and representable TLS/WS/HTTP/obfs settings. Client labels do not determine the returned format. Full profiles contribute no global listeners, rules, DNS, MITM, or scripts. Populated remote node providers and node includes/conditions are errors, never fetched. Only exact parameter-free `DIRECT = direct` / `REJECT = reject` declarations in an explicit `[Proxy]` section are ignored as non-node policy. A profile with no static nodes is not successful empty input.
- A URI list may have one leading bounded `STATUS=` informational header before unindented entries; no metadata is saved. Indented URI-looking text below a STATUS-looking YAML key remains document scalar data, not standalone nodes. Different SNI URI aliases are accepted only for byte-identical decoded values; duplicate occurrences of a single key and conflicting aliases remain errors.
- Protocols: VLESS, VMess (base64 v2 JSON and UUID authority URI), Trojan, Shadowsocks (SIP002, legacy full base64, plaintext escaped userinfo), Hysteria 1/2, TUIC v4/v5, SSH, SOCKS5, and HTTP/HTTPS. Standard `hy`, `hy1`, `hy2`, `socks`, `socks5h`, `socks5s`, and `shadowsocks` aliases are recognized.
- TLS SNI, explicit insecure setting, ALPN, certificate SHA-256 pin, verification hostname, supported uTLS fingerprints, Reality public key/short ID, WS headers/path/early data, and gRPC service name. Native/URI VMess and VLESS HTTP and H2 transports are supported where representable.
- Shadowsocks built-in `obfs` and WebSocket `v2ray-plugin`, SS2022 key validation, UDP-over-TCP. Hysteria hopping/rates/salamander and TUIC congestion/relay options are mapped explicitly.
- SSH password and inline RSA/ECDSA/Ed25519 keys: PKCS1, SEC1, PKCS8, unencrypted OpenSSH; legacy encrypted PEM with passphrase. Host public key pins and supported algorithm lists are validated.

## Inactive TLS defaults

When optional TLS is off, a type-checked `skip-cert-verify: false` and an exactly empty supported SNI field are equivalent to omission. They are consumed, not retained in the canonical output; the TLS switch, credentials, plaintext transport and old fingerprints stay unchanged. Plugin verification and disabled sing-box TLS receive the same narrow handling (disabled uTLS with empty fingerprint and Reality with only its disabled flag are inert). This is not a general false/empty/null filter: active settings, unknown fields, invalid types, nonempty SNI, pins, ALPN and Reality requests still fail. Nonempty `servername` is not dropped because the core can use it as gRPC authority even without TLS. Enabled TLS retains the existing stricter verification rules.

## Same-document relay references

The Clash document layer indexes all entries before resolving `dialer-proxy`, so forward references and Unicode/emoji names work without reordering the returned nodes. Matching is exact and case-sensitive; labels are not trimmed, slugged, or inferred from generated default names. Unique names with equivalent normalized configs are valid aliases and produce identical recursive fingerprints. Duplicate explicit proxy names are rejected, even when their configs are equivalent, rather than picking a definition by document order.

A resolved node carries `ParsedNode.DialerProxy` (the original reference label, public metadata) and `ParsedNode.DialerFingerprint` (the upstream's complete recursive identity, excluded from JSON). `Config` never carries `dialer-proxy`; `normalizeNative` and its unsafe-option allowlist remain strict. Persistence/application code must resolve this metadata to stable node IDs and recompute identities from that graph when restoring data; serializing a parsed node alone deliberately does not preserve its resolved fingerprint. No importer-level direct fallback exists.

Only valid native targets in the current document are eligible. Missing/invalid targets, invalid dependencies, self-references, cycles, and chains longer than **16 dependency edges** are errors on the affected entries. The iterative resolver also detects long cycles without recursive stack growth. A valid suffix of an overlong chain remains importable. Builtins (`DIRECT`, `REJECT`, `GLOBAL`, `REJECT-DROP`, `PASS`, `COMPATIBLE`) and named proxy groups are explicitly rejected as targets, including labels that collide with a native proxy; groups are never flattened or treated as select/fallback aliases. URI delegation, sing-box detours, other imports, stored nodes, and external providers cannot supply a target.

## Safety and limits

No resolver, HTTP client, filesystem access, subprocess, or network side effect exists in production parser code. Importing a URL parses a proxy definition; it never downloads a subscription. All unrecognized node/transport/TLS options are errors. Fields are copied only through consuming allowlists, not by forwarding source maps. Errors never echo submitted values.

Limits: 5 MiB input, 5000 entries, 32 structural levels, 250000 YAML syntax values, four base64 layers, and 16 relay dependency edges. Each textual field is bounded; imported URI credentials must be valid UTF-8 and are percent-decoded once. Large single lines are not subject to the default 64 KiB scanner limit.

`Errors` contains at most 128 diagnostic details, in source order with the original entry line numbers. `TotalErrors` counts every detected issue; `ErrorsTruncated` is true only when that total exceeds the detail cap. Reaching 128 errors does **not** stop parsing or drop later valid siblings/dependency targets in URI, base64, Clash, or sing-box imports. Structural/input limits still apply. Callers must display the total, not mistake `len(Errors)` for an uncapped count, and treat any nonzero total as a partial/failed refresh.

Duplicate keys, YAML anchors/aliases/merge keys/custom tags, multi-document YAML, trailing JSON, malformed scalars, file-backed keys/certificates, custom executables/plugins/scripts, external providers, arbitrary detours/delegation outside the same-document relay path, socket/interface overrides, realm discovery, and pseudo-outbounds are rejected. Empty explicit arrays return zero nodes and no errors; the caller must implement its own empty/shrink subscription protection. Partial results must never be mistaken for a complete successful subscription refresh.

**Ambiguous client defaults:** Surge and Surfboard share the same keyed Trojan/VMess grammar but document different omitted UDP and VMess AEAD defaults. UniProxy cannot infer the originating app from text/URL. Static-client extraction conservatively defaults to TCP-only unless `udp-relay=true` / the documented equivalent is explicit; this is not full Surge-default equivalence. Keyed VMess requires an explicit supported AEAD/alter-ID choice. Do not assume every client setting is reproduced merely because a node parses. For non-QX VMess/VLESS TLS WebSocket entries with a different HTTP Host, an explicit SNI is required: ambiguous omitted/empty SNI must not let the native core silently change the certificate identity. QX documented obfs-host/SNI mapping is preserved. Trojan WS with an explicit TLS name different from its endpoint also requires an explicit HTTP Host; requested SS obfs/plugin modes require a nonempty source host instead of inheriting undocumented core defaults. Certificate pins or verification-name overrides combined with disabled verification are rejected as contradictory.

Static-client fields are parsed with a bounded double-quote-aware tokenizer (256 KiB per node line, 64 KiB per field, 128 fields); credentials are not URL-decoded. Unknown/duplicate options and ambiguous inline comments fail rather than truncate a secret. Bare lists have no relay semantics; node delegation options are rejected. VMess handshake defaults differ by dialect, and unrepresentable legacy handshakes, TLS SPKI pins, custom TLS versions and transport modifiers are not silently substituted. Source-owned scripts/rules in full profiles are ignored as data, not executed.

## Deliberate unsupported variants

- XHTTP, KCP/QUIC obfuscation, HTTPUpgrade, ShadowTLS/Restls/JLS, ECH, custom TLS versions/ciphers/CAs/client certificates, unsupported multiplexing, and unknown native/sing-box options.
- Hysteria fake-TCP/wechat-video, Hysteria2 gecko/realm discovery, URI-only variant extensions that cannot be represented exactly.
- sing-box transports other than WS/gRPC; UDP-only restrictions; HTTP path overrides or multi-value proxy headers; network-specific dial settings.
- SSH bcrypt-encrypted OpenSSH/encrypted PKCS8, DSA, authorized-key options, and SSH certificate algorithms. Empty SSH host pin lists are rejected rather than silently disabling verification. Omitted pins retain standard core password/SSH behavior (no host verification); callers needing pinning should supply `host-key`.
- Non-ASCII hostnames require punycode; scoped IPv6 zone identifiers are rejected.

Pinned Mihomo v1.19.32 source was audited for native mapping correctness. Reality + WebSocket is rejected because the core ignores Reality in that transport; Reality certificate-pin/name/ALPN overrides are rejected when the core ignores them. TUIC `disable-sni: true` is rejected because the core also disables certificate verification. ALPN configurations overridden by WS/H2/gRPC are rejected, not silently dropped. Hysteria rates are canonicalized to integral `Bps` to avoid invalid core rate syntax becoming unlimited. These are explicit errors, not silent fallbacks.

## Tests

```powershell
$env:GOPROXY = 'off'
go test -mod=readonly -count=1 -cover ./internal/importer
go vet -mod=readonly ./internal/importer
go test -mod=readonly ./internal/importer -run='^$' -fuzz='^FuzzParseNeverPanics$' -fuzztime=30s -parallel=2
go test -mod=readonly ./internal/importer -run='^$' -fuzz='^FuzzSSHKeysNeverPanic$' -fuzztime=30s -parallel=2
```

Tests cover every protocol in URI/native/sing-box formats, equivalent normalization, preserved escaped credentials, IPv6, nested maps/transports/security, malformed variants, duplicate keys, credential-safe diagnostics, limits, deterministic concurrency, fingerprint identities, real inline key structure validation, and fuzz invariants. Relay regressions include forward references, Unicode labels, equivalent aliases, recursive credential/option changes, unchanged direct hashes, source ordering/lines, graph failures, 16/17-edge boundaries in both input orders, and a 4999-node cycle with a surviving sibling. A generated synthetic subscription covers 318 VLESS entries with 168 relay references; no private subscription data or credentials are included. Error-cap regressions retain valid siblings after 128/129/1000 failures and verify public total/truncation metadata.

### Execution results in this workspace

- Relay update: `go test -mod=readonly -count=1 -cover ./internal/importer` passed offline, 92.1% statement coverage (2.076 seconds on the final run).
- `go vet -mod=readonly ./internal/importer`: passed.
- Relay-update parser fuzz, 30 seconds / two workers: passed, 168795 executions including graph seeds and error-total/native-config invariants. The focused relay/error regression suite also passed with `-count=3`.
- SSH key fuzz, 30 seconds / two workers: passed, 814857 executions.
- `go test -mod=readonly -race ./internal/importer`: unavailable here: CGO is disabled and no C compiler is installed on PATH. The concurrency unit test passed, but this is not a race-detector substitute.
- Real-core execution was not performed: no pinned core binary was supplied to this task. Mapping was checked against pinned native source; core integration/connectivity remains a separate validation step.

## Mapping references

- [Mihomo protocol and TLS documentation](https://wiki.metacubex.one/en/config/proxies/) (consulted through Context7).
- [Pinned v1.19.32 outbound structs](https://github.com/MetaCubeX/mihomo/tree/v1.19.32/adapter/outbound).
- [Pinned Hysteria bandwidth parser](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/common/utils/mbps.go).
