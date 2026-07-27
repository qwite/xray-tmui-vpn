# xray-tmui-vpn

A terminal UI VPN client prototype for connecting to VLESS servers through
embedded [xray-core](https://github.com/XTLS/Xray-core).

The app uses:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI.
- `github.com/xtls/xray-core/core` to run Xray in-process.
- Xray's JSON config decoder instead of shelling out to an `xray` binary.

## Current Features

- Configure a VLESS outbound from the terminal.
- Import a `vless://` share link into editable connection fields.
- Choose `none`, `tls`, or `reality` stream security.
- Start and stop an embedded xray-core instance.
- Expose local SOCKS and HTTP proxy inbounds.
- Enable macOS and Windows system HTTP/HTTPS/SOCKS proxies while connected.
- Keep the VPN session alive in a background daemon after the TUI exits.
- Switch to a connected dashboard with profile, status, traffic, and log panes.
- Capture xray access/error logs inside the TUI instead of printing below it.
- Show the generated Xray JSON config from the UI.

## Run

Go is required locally.

```bash
go mod tidy
go run ./cmd/xray-tmui-vpn
go run ./cmd/xray-tmui-vpn --version
```

Prebuilt archives are published on the
[GitHub Releases](https://github.com/qwite/xray-tmui-vpn/releases) page.

## Build

The application is a single executable and does not require a separate Xray
installation.

```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o xray-tmui-vpn-darwin-arm64 ./cmd/xray-tmui-vpn
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -o xray-tmui-vpn-darwin-amd64 ./cmd/xray-tmui-vpn
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o xray-tmui-vpn-windows-amd64.exe ./cmd/xray-tmui-vpn
```

## Release

Pushing a semantic version tag such as `v0.1.0-alpha` runs GoReleaser through
GitHub Actions. The workflow tests the project, builds macOS amd64/arm64 and
Windows amd64 archives, generates SHA-256 checksums, and publishes a GitHub
prerelease for tags with a prerelease suffix.

Validate the release configuration locally without publishing:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

## Controls

- `tab` / `shift+tab`: move between fields; compact windows scroll automatically
- `f2`: cycle stream security
- `enter`: connect or disconnect
- `f3`: toggle generated config view
- `f4`: import the pasted `vless://` link
- `f5`: export hidden logs from the dashboard
- `e`: edit the saved profile from the dashboard
- `esc` / `ctrl+c`: quit the TUI without disconnecting

## Notes

This is a proxy-mode client. It starts local SOCKS/HTTP listeners and routes
traffic through the configured VLESS outbound. On macOS and Windows, it also
points the current user's system HTTP/HTTPS/SOCKS proxy settings at those
listeners while connected and restores the previous settings on disconnect.
It does not create a system TUN interface yet.

The Windows integration updates the current user's WinINet proxy settings.
Applications and Windows services that ignore those settings or use WinHTTP
directly will not be routed through the client.

When the terminal is shorter than the complete connection form, the TUI keeps
status and security visible and shows a focus-following field viewport.

The TUI starts a background daemon when connecting. Closing the terminal or
quitting with `esc` / `ctrl+c` leaves that daemon running, so the VPN remains
connected. Launching the TUI again reads the daemon state and shows the real
connected status. Use `enter` from the dashboard to disconnect and stop the
daemon.

The daemon considers the connection ready when Xray's local HTTP proxy starts
listening. Set `XRAY_TMUI_VPN_READINESS_URL` to an HTTP URL to additionally
require an end-to-end request through the VLESS outbound during startup.

For a full VPN experience, the next step is adding a TUN inbound and platform
specific routing/DNS setup.

Architecture, current engineering constraints, and the working roadmap are
tracked in [PROJECT_CONTEXT.md](PROJECT_CONTEXT.md).
