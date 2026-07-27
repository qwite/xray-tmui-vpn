# Project Context

Last updated: 2026-07-27

## Purpose

`xray-tmui-vpn` is a Go prototype of a terminal VPN client for VLESS
connections. It embeds `xray-core` in the application process and uses Bubble
Tea for the terminal UI.

The current implementation is a proxy-mode client, not a full system VPN. Xray
listens on local SOCKS and HTTP ports and sends that traffic through a VLESS
outbound. On macOS and Windows, the daemon also points the current user's
system HTTP, HTTPS, and SOCKS proxy settings at those local listeners.

## Current Product Behavior

- A user can enter VLESS connection fields or import a `vless://` share link.
- Supported stream security modes are `none`, `tls`, and `reality`.
- The generated Xray JSON configuration can be inspected in the TUI.
- Connecting saves one active profile and starts a detached daemon.
- The daemon owns the embedded Xray instance, so closing the TUI does not end
  the connection.
- Reopening the TUI reads daemon state and returns to the connected dashboard.
- The dashboard shows connection state, profile data, traffic counters, and
  captured Xray logs.
- Disconnecting from the dashboard asks the daemon to stop gracefully, stops
  Xray, and restores the system proxy settings captured when it connected.

Default local listeners:

- SOCKS: `127.0.0.1:10808`
- HTTP: `127.0.0.1:10809`

## Architecture

The project builds one executable with two modes:

```text
xray-tmui-vpn          -> Bubble Tea TUI
xray-tmui-vpn daemon   -> detached connection owner
```

Connection flow:

```text
TUI
  -> validates and saves the active profile
  -> starts a detached copy of the executable in daemon mode
  -> daemon starts embedded xray-core
  -> daemon enables system proxies on macOS or Windows
  -> daemon verifies the HTTP proxy with a readiness request
  -> daemon writes state, counters, and logs once per second
  -> TUI polls that state for the dashboard
```

Package responsibilities:

- `cmd/xray-tmui-vpn`: executable entry point and mode selection.
- `internal/tui`: Bubble Tea model, form, dashboard, config view, and keyboard
  handling.
- `internal/daemon`: daemon lifecycle, readiness check, state persistence,
  signals, and TUI-facing status operations.
- `internal/xray`: VLESS link parsing, runtime configuration validation, Xray
  JSON generation, embedded Xray lifecycle, traffic counters, and log capture.
- `internal/profile`: persistence of the single active profile and log export.
- `internal/systemproxy`: macOS `networksetup` integration, Windows per-user
  Internet Settings integration, and a no-op manager on other Unix platforms.

## Persistent State

By default, profile and daemon state use the OS user config directory under
`xray-tmui-vpn`; daemon and exported logs use the OS user cache directory under
the same name.

Files:

- `profile.json`: the single saved profile.
- `state.json`: daemon PID, lifecycle state, active profile, counters, recent
  logs, errors, and timestamps.
- `stop.json`: short-lived PID-scoped request for graceful daemon shutdown.
- `daemon.log`: daemon stdout and stderr.
- `xray-tmui-vpn.log`: logs exported from the dashboard.

`XRAY_TMUI_VPN_CONFIG_DIR` overrides both directories, primarily for tests.
`XRAY_TMUI_VPN_READINESS_URL` overrides the default readiness target
`http://example.com/`.

Secrets such as the VLESS UUID and Reality public key are currently stored in
plain JSON with file mode `0600`.

## Important Constraints

- There is no TUN inbound, route management, or VPN-specific DNS handling yet.
- Only TCP stream settings are generated.
- Only one saved/active profile is modeled; there is no profile collection.
- macOS and Windows have current-user system proxy integration.
- The Windows integration affects WinINet proxy settings. Software using
  WinHTTP, its own proxy configuration, or raw sockets may bypass it.
- Other Unix-like platforms can use the local SOCKS/HTTP listeners, but their
  system proxy settings are not changed.
- System proxy restoration depends on graceful daemon shutdown. A crash or
  forced kill can leave system proxy settings enabled on macOS or Windows.
- The daemon readiness check requires an HTTP request through the configured
  proxy to succeed within 20 seconds.

## Development

Requirements: Go 1.26 or a compatible toolchain for the version declared in
`go.mod`.

```bash
go mod tidy
go run ./cmd/xray-tmui-vpn
go test ./...
```

`xray-tmui-vpn --version` reports the application version, source commit, and
build timestamp. Local builds report `dev`; GoReleaser injects release metadata
through linker flags.

Tests currently cover VLESS link parsing, profile persistence and log export,
daemon state and stop-request helpers, readiness URL override, TUI
interaction/state loading, macOS proxy output parsing, and Windows proxy server
formatting. CI runs tests and builds natively on macOS and Windows. The suite
does not provide an end-to-end connection test against a real VLESS server.

## Release Engineering

- CI runs tests and builds natively on macOS and Windows for pushes to `main`
  and pull requests.
- Tags matching `v*` trigger the release workflow.
- GoReleaser builds macOS amd64/arm64 and Windows amd64 with CGO disabled.
- macOS artifacts use `tar.gz`; Windows uses ZIP.
- Each release includes `checksums.txt` with SHA-256 hashes.
- Semantic-version prerelease tags such as `v0.1.0-alpha` create GitHub
  prereleases.
- Release binaries are not code-signed or notarized yet.

## Next Direction

The next major product milestone is a real VPN mode:

1. Add a TUN inbound using established Xray/platform facilities.
2. Add platform-specific route lifecycle management.
3. Add DNS configuration and reliable restoration.
4. Make startup and shutdown transactional so partial failures do not leave
   routes, DNS, or proxies behind.
5. Add integration tests around connection lifecycle and recovery.

Before implementing that milestone, decide whether proxy mode remains a
separate selectable mode or is replaced by TUN mode.

## Maintenance

Update this file when the product boundary, connection lifecycle, persistence
format, platform support, or primary roadmap changes. Keep detailed controls
and user-facing setup in `README.md`; keep architectural decisions and current
engineering constraints here.
