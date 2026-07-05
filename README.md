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
- Enable macOS system HTTP/HTTPS/SOCKS proxies while connected.
- Keep the VPN session alive in a background daemon after the TUI exits.
- Switch to a connected dashboard with profile, status, traffic, and log panes.
- Capture xray access/error logs inside the TUI instead of printing below it.
- Show the generated Xray JSON config from the UI.

## Run

Go is required locally.

```bash
go mod tidy
go run ./cmd/xray-tmui-vpn
```

## Controls

- `tab` / `shift+tab`: move between fields
- `f2`: cycle stream security
- `enter`: connect or disconnect
- `f3`: toggle generated config view
- `f4`: import the pasted `vless://` link
- `f5`: export hidden logs from the dashboard
- `e`: edit the saved profile from the dashboard
- `esc` / `ctrl+c`: quit the TUI without disconnecting

## Notes

This is a proxy-mode client. It starts local SOCKS/HTTP listeners and routes
traffic through the configured VLESS outbound. On macOS, it also points the
system HTTP/HTTPS/SOCKS proxy settings at those listeners while connected and
restores the previous disabled proxy state on disconnect. It does not create a
system TUN interface yet.

The TUI starts a background daemon when connecting. Closing the terminal or
quitting with `esc` / `ctrl+c` leaves that daemon running, so the VPN remains
connected. Launching the TUI again reads the daemon state and shows the real
connected status. Use `enter` from the dashboard to disconnect and stop the
daemon.

For a full VPN experience, the next step is adding a TUN inbound and platform
specific routing/DNS setup.
