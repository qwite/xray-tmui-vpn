package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qwites/xray-tmui-vpn/internal/profile"
	"github.com/qwites/xray-tmui-vpn/internal/systemproxy"
	"github.com/qwites/xray-tmui-vpn/internal/xray"
)

const (
	statusConnecting    = "connecting"
	statusConnected     = "connected"
	statusDisconnecting = "disconnecting"
	statusDisconnected  = "disconnected"
	statusError         = "error"

	readinessURLEnv  = "XRAY_TMUI_VPN_READINESS_URL"
	readinessTimeout = 20 * time.Second
)

type State struct {
	PID           int                `json:"pid"`
	Status        string             `json:"status"`
	Profile       profile.Profile    `json:"profile"`
	Version       string             `json:"version"`
	UplinkBytes   int64              `json:"uplinkBytes"`
	DownlinkBytes int64              `json:"downlinkBytes"`
	LogLines      []string           `json:"logLines"`
	Error         string             `json:"error,omitempty"`
	StartedAt     time.Time          `json:"startedAt,omitempty"`
	UpdatedAt     time.Time          `json:"updatedAt"`
	Config        xray.RuntimeConfig `json:"-"`
}

type stopRequest struct {
	PID int `json:"pid"`
}

func Run() error {
	ignoreTerminalHangup()

	savedProfile, ok, err := profile.Load()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no saved profile")
	}

	client := xray.NewClient()
	proxy := systemproxy.NewManager()
	state := State{
		PID:       os.Getpid(),
		Status:    statusConnecting,
		Profile:   savedProfile,
		Version:   xray.Version(),
		StartedAt: time.Now(),
	}
	if err := saveState(state); err != nil {
		return err
	}

	if err := client.Start(savedProfile.Config); err != nil {
		state.Status = statusError
		state.Error = err.Error()
		state.UpdatedAt = time.Now()
		_ = saveState(state)
		return err
	}
	if err := proxy.Enable(savedProfile.Config.SOCKSPort, savedProfile.Config.HTTPPort); err != nil {
		_ = client.Stop()
		state.Status = statusError
		state.Error = err.Error()
		state.UpdatedAt = time.Now()
		_ = saveState(state)
		return err
	}

	if err := waitForProxyReady(savedProfile.Config, client, state); err != nil {
		state = applySnapshot(state, client.Snapshot())
		state.Status = statusError
		state.Error = err.Error()
		state.UpdatedAt = time.Now()
		_ = saveState(state)
		_ = proxy.Disable()
		_ = client.Stop()
		return err
	}

	state = applySnapshot(state, client.Snapshot())
	state.Status = statusConnected
	state.UpdatedAt = time.Now()
	if err := saveState(state); err != nil {
		_ = proxy.Disable()
		_ = client.Stop()
		return err
	}

	signals := make(chan os.Signal, 1)
	notifyStop(signals)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if requested, _ := stopRequested(os.Getpid()); requested {
				_ = clearStopRequest()
				return shutdown(client, proxy, state)
			}
			state = applySnapshot(state, client.Snapshot())
			state.Status = statusConnected
			state.UpdatedAt = time.Now()
			_ = saveState(state)
		case <-signals:
			return shutdown(client, proxy, state)
		}
	}
}

func Start(savedProfile profile.Profile) (State, error) {
	if strings.TrimSpace(savedProfile.Name) == "" {
		savedProfile.Name = defaultProfileName(savedProfile.Config)
	}
	if err := profile.Save(savedProfile); err != nil {
		return State{}, err
	}

	if state, active, err := Status(); err != nil {
		return State{}, err
	} else if active {
		return state, nil
	}
	if err := clearStopRequest(); err != nil {
		return State{}, err
	}

	executable, err := os.Executable()
	if err != nil {
		return State{}, err
	}

	logFile, err := openDaemonLog()
	if err != nil {
		return State{}, err
	}
	defer logFile.Close()

	cmd := exec.Command(executable, "daemon")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = os.Environ()
	cmd.SysProcAttr = detachedProcessAttributes()
	if err := cmd.Start(); err != nil {
		return State{}, err
	}

	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return State{}, err
	}

	deadline := time.Now().Add(readinessTimeout + 5*time.Second)
	for time.Now().Before(deadline) {
		state, active, err := Status()
		if err != nil && !os.IsNotExist(err) {
			return State{}, err
		}
		if state.PID == pid && state.Status == statusError {
			return state, errors.New(state.Error)
		}
		if state.PID == pid && active && state.Status == statusConnected {
			return state, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return State{}, fmt.Errorf("daemon did not report connected state")
}

func Stop() (State, error) {
	state, active, err := Status()
	if err != nil {
		return State{}, err
	}
	if !active {
		if IsError(state) && strings.TrimSpace(state.Error) != "" {
			return state, errors.New(state.Error)
		}
		return state, nil
	}

	if err := requestDaemonStop(state.PID); err != nil {
		return state, err
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		next, active, err := Status()
		if err != nil {
			return next, err
		}
		if !active {
			if IsError(next) && strings.TrimSpace(next.Error) != "" {
				return next, errors.New(next.Error)
			}
			return next, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	process, err := os.FindProcess(state.PID)
	if err != nil {
		return state, err
	}
	if err := terminateProcess(process); err != nil {
		return state, err
	}

	err = errors.New("daemon did not stop gracefully; forced termination may have left system proxy settings enabled")
	state.PID = 0
	state.Status = statusError
	state.Error = err.Error()
	_ = saveState(state)
	_ = clearStopRequest()
	return state, err
}

func Status() (State, bool, error) {
	state, ok, err := loadState()
	if err != nil || !ok {
		return state, false, err
	}

	active := state.PID > 0 && processAlive(state.PID)
	if !active && state.Status != statusDisconnected && state.Status != statusError {
		state.PID = 0
		state.Status = statusDisconnected
		state.UpdatedAt = time.Now()
		_ = saveState(state)
	}

	return state, active, nil
}

func SnapshotFromState(state State) xray.Snapshot {
	return xray.Snapshot{
		Version:       valueOr(state.Version, xray.Version()),
		UplinkBytes:   state.UplinkBytes,
		DownlinkBytes: state.DownlinkBytes,
		LogLines:      state.LogLines,
	}
}

func IsConnected(state State) bool {
	return state.Status == statusConnected
}

func IsConnecting(state State) bool {
	return state.Status == statusConnecting
}

func IsDisconnecting(state State) bool {
	return state.Status == statusDisconnecting
}

func IsError(state State) bool {
	return state.Status == statusError
}

func DisplayStatus(state State, active bool) string {
	switch {
	case active && state.Status == statusConnecting:
		return "Connecting..."
	case active && state.Status == statusDisconnecting:
		return "Disconnecting..."
	case active && state.Status == statusConnected:
		return "Connected"
	case state.Status == statusError && strings.TrimSpace(state.Error) != "":
		return state.Error
	case state.Status == statusDisconnected:
		return "Disconnected"
	default:
		return "Ready"
	}
}

func statePath() (string, error) {
	dir, err := profile.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func stopRequestPath() (string, error) {
	dir, err := profile.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "stop.json"), nil
}

func openDaemonLog() (*os.File, error) {
	dir, err := profile.DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}

func loadState() (State, bool, error) {
	path, err := statePath()
	if err != nil {
		return State{}, false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false, nil
		}
		return State{}, false, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func saveState(state State) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func requestDaemonStop(pid int) error {
	path, err := stopRequestPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(stopRequest{PID: pid})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func stopRequested(pid int) (bool, error) {
	path, err := stopRequestPath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var request stopRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return false, err
	}
	return request.PID == pid, nil
}

func clearStopRequest() error {
	path, err := stopRequestPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return processRunning(process)
}

func shutdown(client *xray.Client, proxy *systemproxy.Manager, state State) error {
	state.Status = statusDisconnecting
	state.UpdatedAt = time.Now()
	_ = saveState(state)

	state = applySnapshot(state, client.Snapshot())
	proxyErr := proxy.Disable()
	clientErr := client.Stop()
	cleanupErr := errors.Join(proxyErr, clientErr)
	state.PID = 0
	if cleanupErr != nil {
		state.Status = statusError
		state.Error = "disconnect cleanup: " + cleanupErr.Error()
	} else {
		state.Status = statusDisconnected
		state.Error = ""
	}
	state.UpdatedAt = time.Now()
	stateErr := saveState(state)
	return errors.Join(cleanupErr, stateErr)
}

func waitForProxyReady(config xray.RuntimeConfig, client *xray.Client, state State) error {
	deadline := time.Now().Add(readinessTimeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if err := probeLocalProxy(config.HTTPPort); err != nil {
			lastErr = err
		} else if targetURL := readinessURL(); targetURL != "" {
			if err := probeHTTPProxy(config.HTTPPort, targetURL); err != nil {
				lastErr = err
			} else {
				return nil
			}
		} else {
			return nil
		}

		state = applySnapshot(state, client.Snapshot())
		state.Status = statusConnecting
		state.Error = ""
		state.UpdatedAt = time.Now()
		_ = saveState(state)
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr == nil {
		return fmt.Errorf("proxy readiness check timed out")
	}
	return fmt.Errorf("proxy readiness check timed out: %w", lastErr)
}

func probeLocalProxy(httpPort int) error {
	connection, err := net.DialTimeout(
		"tcp",
		net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", httpPort)),
		2*time.Second,
	)
	if err != nil {
		return err
	}
	return connection.Close()
}

func probeHTTPProxy(httpPort int, targetURL string) error {
	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", httpPort))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   2 * time.Second,
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return fmt.Errorf("readiness probe returned HTTP %d", response.StatusCode)
	}
	return nil
}

func readinessURL() string {
	return strings.TrimSpace(os.Getenv(readinessURLEnv))
}

func applySnapshot(state State, snapshot xray.Snapshot) State {
	state.Version = valueOr(snapshot.Version, xray.Version())
	state.UplinkBytes = snapshot.UplinkBytes
	state.DownlinkBytes = snapshot.DownlinkBytes
	state.LogLines = trimLogLines(snapshot.LogLines)
	return state
}

func trimLogLines(lines []string) []string {
	const maxLogs = 500
	if len(lines) <= maxLogs {
		return lines
	}
	return lines[len(lines)-maxLogs:]
}

func defaultProfileName(config xray.RuntimeConfig) string {
	return fmt.Sprintf("%s-%s", strings.TrimSpace(config.ServerAddress), config.Security)
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
