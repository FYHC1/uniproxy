# Managed Mihomo core

This package implements the core interface in [the implementation contract](../../docs/implementation-contract.md). The only non-standard-library dependency is the repository's existing YAML encoder. Native proxy maps are copied from the importer; labels never become routing identifiers.

## Guarantees

- Exactly Mihomo **v1.19.32** is accepted. Other versions are not assumed to support `hash-key: in-user` and fail with `ErrUnsupportedVersion`. Missing binaries return `ErrUnavailable`.
- Binary discovery checks authoritative `Options.BinaryPath`; without an override it checks `Options.DataDir/mihomo[.exe]`, working-directory `bin/mihomo[.exe]`, the UniProxy executable directory, then PATH. Invalid/incompatible first candidates do not silently fall through. The application passes its platform-resolved data directory plus `core`; standalone manager default remains `data/core`.
- Stable IDs become `n-ID`, `p-ID`, and `l-ID`. Node and pool membership is ordered by ID. Renaming entities does not change their compiled configuration.
- Native `url-test` and `load-balance` groups implement round-robin, consistent-hashing, and sticky-sessions. Every group explicitly uses `empty-fallback: REJECT`. Empty pools contain only `REJECT`. Stale/disabled targets bind to `REJECT`. Global and private-probe rules end with `MATCH,REJECT`.
- Each public listener binds directly to its target through native `proxy`. The private mixed probe uses node-specific authenticated usernames and HMAC-derived passwords with isolated `IN-USER` sub-rules; public listener usernames cannot select probe routes.
- Controller/probe addresses are random literal loopback endpoints. Both have independent random secrets. The controller is never proxied through the application API.
- Configuration validation and startup use `mihomo -t -f -` and `mihomo -f -` respectively. Decrypted native YAML goes over child stdin, not command arguments, environment, or persistent plaintext files. Reload/rollback payloads stay in memory. Ambient `CLASH_*`/`MIHOMO_*` overrides are stripped from the child environment.
- Apply is serialized: compile, verify pinned version, native validation, preflight bind checks, native reload, authenticated protocol checks, commit. Failed changes restore the previous live configuration. Failed rollback can restart the previous config; if recovery fails, the child is stopped rather than leaving a partially applied setup.
- Native logs are bounded in memory. Raw configuration errors/controller bodies are withheld because they can contain credentials. Application callbacks receive fixed, secret-free operational messages.

## Honest limitations

Normal reload uses `PUT /configs?force=true`, never `DELETE /connections` and never a process restart. Unchanged listener accepts and established TCP connections can survive. Changed ports/auth briefly recreate accept sockets; port swaps use a staged native reload. Native health/group state, sticky-session choices, and UDP mappings can reset. Removing or disabling a target rejects **new** flows; already established streams may continue until their normal close. Stop and restart recovery necessarily close connections.

Mihomo v1.19.32's native SOCKS UDP handler **does not authenticate individual packets or attach the inbound username**, even when TCP authentication is configured. Therefore:

- UDP listeners must bind loopback; public/LAN UDP binds are rejected rather than exposing an unauthenticated relay.
- HTTP listeners cannot enable UDP.
- UDP listeners require a compatible node or a UDP-filtered pool.
- Listeners targeting `hash-key: in-user` pools must use authentication and TCP-only mode. A user hash is not falsely promised for UDP.
- Loopback UDP remains unauthenticated at the packet layer; local processes can send to it.

A URL-test uses the core's real `/proxies/{stable-tag}/delay` API, with expected status 200–399. The native API reports a rounded 0ms result as HTTP503; extremely fast loopback targets can therefore fail and should use a suitable real test endpoint. The pinned API parses timeouts as a signed 16-bit millisecond integer, so latency timeouts are capped at **32,767 ms**.

Speed tests perform real downloads through the node-authenticated internal proxy, use a fresh transport per test, and bound reads to the requested byte limit. Maximums: **100 MiB**, **2 minutes**, **5 redirects**, and **64 KiB response headers**. No direct or environment-proxy fallback exists. Cancellation interrupts body reads; empty or failed transfers never produce a successful/fabricated throughput value.

## Install and test

Start UniProxy without installing a separate command. The authenticated WebUI prompts when the core is missing, and both operations/settings tabs expose installation and rediscovery. The Go management API starts an asynchronous task through this Manager; [coreinstall](<../coreinstall/install.go>) contains the verified downloader.

- `InstallationStatus()` returns transient task state and local discovery without network I/O or subprocesses.
- `PrepareInstall(ctx)` atomically reserves a missing destination; running/applying/busy/existing/unwritable-as-PATH-name targets are rejected.
- `RunInstall(ctx)` downloads in the background without holding the lifecycle mutex, then verifies the installed version. Apply/Validate are excluded by the reservation.
- App shutdown cancels and waits. A request disconnect or closing the modal does not cancel an accepted task.
- Install success never starts proxying or changes configuration revision. App pauses automatic apply until explicit runtime Apply; restarting the app retains existing startup-apply behavior.
- An interrupted post-install version probe keeps the verified file. Explicit Inspect can recover the install status without redownloading/replacing; ordinary probes are capped at20s, first post-install probes at60s, within the5min installation budget.

The downloader accepts only Windows/Linux amd64-compatible and arm64 pinned v1.19.32 assets. It checks HTTPS origins on every redirect, public DNS/numeric-pinned dialing/TLS, exact official release tag and URL, advertised size, GitHub SHA256 and built-in SHA256 before extraction. Archive paths, symlinks, ambiguous executables, CRC failures and oversized expansion are rejected. The extracted staging file and archive are closed before cleanup/publication (Windows-safe); same-directory `os.Link` provides atomic **no-replace** publication even when two processes race. Filesystems without hard links fail safely. No public version/URL/force overrides or downloaded scripts exist.

```sh
go test ./internal/core ./internal/coreinstall
go vet ./internal/core ./internal/coreinstall
```

Real integration is opt-in and uses only local HTTP/SOCKS/TCP/UDP fixtures, no public proxy accounts:

```sh
UNIPROXY_TEST_CORE_PATH=/absolute/path/to/mihomo go test ./internal/core -run TestReal -count=1 -v
```

PowerShell:

```powershell
$env:UNIPROXY_TEST_CORE_PATH = (Resolve-Path data/core/mihomo.exe).Path
go test ./internal/core -run TestReal -count=1 -v
```

Coverage includes real HTTP/SOCKS/mixed auth, HTTP and SOCKS native upstreams, UDP datagrams, native group strategies, actual URL delay/download APIs, live TCP preservation across reload/port swaps, and fail-closed empty/deleted targets. Mock child/controller tests additionally cover validation rejection, unavailable/unpinned binaries, failed reload, failed verification, rollback restart, unexpected exit, occupied ports, canceled updates, canceled downloads, and concurrent status/tests.

## Verified upstream references

- [Native listener binding options, v1.19.32](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/listener/inbound/base.go)
- [Native groups and in-user hashing](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/adapter/outboundgroup/loadbalance.go)
- [Explicit empty fallback behavior](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/adapter/outboundgroup/parser.go)
- [Configuration controller API](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/hub/route/configs.go)
- [Core URL-test endpoint](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/hub/route/proxies.go)
- [Native listener reload behavior](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/listener/listener.go)
- [SOCKS UDP authentication limitation](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/listener/socks/udp.go)
- [stdin configuration support](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/main.go)
- [Official pinned release](https://github.com/MetaCubeX/mihomo/releases/tag/v1.19.32)
