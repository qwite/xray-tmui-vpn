//go:build windows

package systemproxy

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsPath          = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	internetOptionRefresh         = 37
	internetOptionSettingsChanged = 39
)

type proxyState struct {
	enabled       uint32
	enabledExists bool
	server        string
	serverExists  bool
}

type Manager struct {
	mu       sync.Mutex
	enabled  bool
	previous proxyState
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Enable(socksPort, httpPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.enabled {
		return nil
	}

	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		internetSettingsPath,
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open Windows internet settings: %w", err)
	}
	defer key.Close()

	previous, err := readProxyState(key)
	if err != nil {
		return err
	}

	server := formatProxyServer(socksPort, httpPort)
	if err := key.SetStringValue("ProxyServer", server); err != nil {
		return fmt.Errorf("set Windows proxy server: %w", err)
	}
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		_ = restoreProxyState(key, previous)
		return fmt.Errorf("enable Windows proxy: %w", err)
	}
	if err := notifyInternetSettingsChanged(); err != nil {
		_ = restoreProxyState(key, previous)
		_ = notifyInternetSettingsChanged()
		return err
	}

	m.previous = previous
	m.enabled = true
	return nil
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return nil
	}

	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		internetSettingsPath,
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open Windows internet settings: %w", err)
	}
	defer key.Close()

	if err := restoreProxyState(key, m.previous); err != nil {
		return err
	}
	if err := notifyInternetSettingsChanged(); err != nil {
		return err
	}

	m.previous = proxyState{}
	m.enabled = false
	return nil
}

func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.enabled
}

func readProxyState(key registry.Key) (proxyState, error) {
	var state proxyState

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	switch {
	case err == nil:
		state.enabled = uint32(enabled)
		state.enabledExists = true
	case !errors.Is(err, registry.ErrNotExist):
		return proxyState{}, fmt.Errorf("read Windows proxy state: %w", err)
	}

	server, _, err := key.GetStringValue("ProxyServer")
	switch {
	case err == nil:
		state.server = server
		state.serverExists = true
	case !errors.Is(err, registry.ErrNotExist):
		return proxyState{}, fmt.Errorf("read Windows proxy server: %w", err)
	}

	return state, nil
}

func restoreProxyState(key registry.Key, state proxyState) error {
	if state.serverExists {
		if err := key.SetStringValue("ProxyServer", state.server); err != nil {
			return fmt.Errorf("restore Windows proxy server: %w", err)
		}
	} else if err := key.DeleteValue("ProxyServer"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("remove Windows proxy server: %w", err)
	}

	if state.enabledExists {
		if err := key.SetDWordValue("ProxyEnable", state.enabled); err != nil {
			return fmt.Errorf("restore Windows proxy state: %w", err)
		}
	} else if err := key.DeleteValue("ProxyEnable"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("remove Windows proxy state: %w", err)
	}

	return nil
}

func formatProxyServer(socksPort, httpPort int) string {
	return fmt.Sprintf(
		"http=127.0.0.1:%d;https=127.0.0.1:%d;socks=127.0.0.1:%d",
		httpPort,
		httpPort,
		socksPort,
	)
}

func notifyInternetSettingsChanged() error {
	wininet := windows.NewLazySystemDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")

	for _, option := range []uintptr{internetOptionSettingsChanged, internetOptionRefresh} {
		result, _, callErr := internetSetOption.Call(0, option, 0, 0)
		if result == 0 {
			if callErr == windows.ERROR_SUCCESS {
				callErr = errors.New("InternetSetOptionW returned false")
			}
			return fmt.Errorf("notify Windows internet settings change: %w", callErr)
		}
	}
	return nil
}
