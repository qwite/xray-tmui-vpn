package daemon

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/qwites/xray-tmui-vpn/internal/profile"
	"github.com/qwites/xray-tmui-vpn/internal/xray"
)

func TestStatusReportsCurrentProcessActive(t *testing.T) {
	t.Setenv("XRAY_TMUI_VPN_CONFIG_DIR", t.TempDir())

	want := State{
		PID:     os.Getpid(),
		Status:  statusConnected,
		Version: xray.Version(),
		Profile: profile.Profile{
			Name: "example-profile",
			Config: xray.RuntimeConfig{
				ServerAddress: "vpn.example.com",
				ServerPort:    443,
				UUID:          "11111111-1111-4111-8111-111111111111",
				ServerName:    "vpn.example.com",
				Security:      "tls",
				SOCKSPort:     10808,
				HTTPPort:      10809,
			},
		},
		StartedAt: time.Now(),
	}
	if err := saveState(want); err != nil {
		t.Fatal(err)
	}

	got, active, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("current process state was not active")
	}
	if got.PID != want.PID {
		t.Fatalf("pid = %d, want %d", got.PID, want.PID)
	}
}

func TestStatusPreservesDaemonError(t *testing.T) {
	t.Setenv("XRAY_TMUI_VPN_CONFIG_DIR", t.TempDir())

	want := State{
		PID:    999999,
		Status: statusError,
		Error:  "failed to start",
	}
	if err := saveState(want); err != nil {
		t.Fatal(err)
	}

	got, active, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("error state should not be active")
	}
	if got.Status != statusError {
		t.Fatalf("status = %q, want %q", got.Status, statusError)
	}
	if got.Error != want.Error {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestStopReturnsPersistedDaemonError(t *testing.T) {
	t.Setenv("XRAY_TMUI_VPN_CONFIG_DIR", t.TempDir())

	want := "disconnect cleanup: restore proxy settings"
	if err := saveState(State{
		Status: statusError,
		Error:  want,
	}); err != nil {
		t.Fatal(err)
	}

	state, err := Stop()
	if err == nil || err.Error() != want {
		t.Fatalf("Stop() error = %v, want %q", err, want)
	}
	if state.Error != want {
		t.Fatalf("state error = %q, want %q", state.Error, want)
	}
}

func TestSnapshotFromState(t *testing.T) {
	state := State{
		Version:       "26.3.27",
		UplinkBytes:   123,
		DownlinkBytes: 456,
		LogLines:      []string{"started"},
	}

	snapshot := SnapshotFromState(state)
	if snapshot.Version != state.Version {
		t.Fatalf("version = %q", snapshot.Version)
	}
	if snapshot.UplinkBytes != state.UplinkBytes {
		t.Fatalf("uplink = %d", snapshot.UplinkBytes)
	}
	if snapshot.DownlinkBytes != state.DownlinkBytes {
		t.Fatalf("downlink = %d", snapshot.DownlinkBytes)
	}
	if len(snapshot.LogLines) != 1 || snapshot.LogLines[0] != "started" {
		t.Fatalf("logs = %#v", snapshot.LogLines)
	}
}

func TestDisplayStatus(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		active bool
		want   string
	}{
		{
			name:   "connecting",
			state:  State{Status: statusConnecting},
			active: true,
			want:   "Connecting...",
		},
		{
			name:   "connected",
			state:  State{Status: statusConnected},
			active: true,
			want:   "Connected",
		},
		{
			name:   "error",
			state:  State{Status: statusError, Error: "probe failed"},
			active: false,
			want:   "probe failed",
		},
		{
			name:   "disconnected",
			state:  State{Status: statusDisconnected},
			active: false,
			want:   "Disconnected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayStatus(tt.state, tt.active); got != tt.want {
				t.Fatalf("DisplayStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadinessURLCanBeOverridden(t *testing.T) {
	t.Setenv(readinessURLEnv, "http://probe.example.test/")

	if got := readinessURL(); got != "http://probe.example.test/" {
		t.Fatalf("readinessURL() = %q", got)
	}
}

func TestReadinessURLIsDisabledByDefault(t *testing.T) {
	t.Setenv(readinessURLEnv, "")

	if got := readinessURL(); got != "" {
		t.Fatalf("readinessURL() = %q, want empty", got)
	}
}

func TestProbeLocalProxyDetectsListeningPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if err := probeLocalProxy(port); err != nil {
		t.Fatalf("probeLocalProxy(%d): %v", port, err)
	}
}

func TestStopRequestMatchesDaemonPID(t *testing.T) {
	t.Setenv("XRAY_TMUI_VPN_CONFIG_DIR", t.TempDir())

	if err := requestDaemonStop(1234); err != nil {
		t.Fatalf("requestDaemonStop: %v", err)
	}

	requested, err := stopRequested(1234)
	if err != nil {
		t.Fatalf("stopRequested: %v", err)
	}
	if !requested {
		t.Fatal("stopRequested() = false for matching PID")
	}

	requested, err = stopRequested(4321)
	if err != nil {
		t.Fatalf("stopRequested for another PID: %v", err)
	}
	if requested {
		t.Fatal("stopRequested() = true for another PID")
	}

	if err := clearStopRequest(); err != nil {
		t.Fatalf("clearStopRequest: %v", err)
	}
	requested, err = stopRequested(1234)
	if err != nil {
		t.Fatalf("stopRequested after clear: %v", err)
	}
	if requested {
		t.Fatal("stopRequested() = true after clear")
	}
}
