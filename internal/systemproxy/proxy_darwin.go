package systemproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const loopbackHost = "127.0.0.1"

type commandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type Manager struct {
	mu       sync.Mutex
	runner   commandRunner
	enabled  bool
	previous map[string]map[proxyKind]proxyState
}

type proxyKind struct {
	label    string
	get      string
	set      string
	setState string
}

type proxyState struct {
	enabled bool
	server  string
	port    int
}

var proxyKinds = []proxyKind{
	{
		label:    "web proxy",
		get:      "-getwebproxy",
		set:      "-setwebproxy",
		setState: "-setwebproxystate",
	},
	{
		label:    "secure web proxy",
		get:      "-getsecurewebproxy",
		set:      "-setsecurewebproxy",
		setState: "-setsecurewebproxystate",
	},
	{
		label:    "SOCKS proxy",
		get:      "-getsocksfirewallproxy",
		set:      "-setsocksfirewallproxy",
		setState: "-setsocksfirewallproxystate",
	},
}

func NewManager() *Manager {
	return &Manager{runner: execRunner{}}
}

func (m *Manager) Enable(socksPort, httpPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.enabled {
		return nil
	}

	services, err := m.listNetworkServices()
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return fmt.Errorf("no enabled macOS network services found")
	}

	previous := make(map[string]map[proxyKind]proxyState, len(services))
	for _, service := range services {
		previous[service] = make(map[proxyKind]proxyState, len(proxyKinds))
		for _, kind := range proxyKinds {
			state, err := m.getProxyState(service, kind)
			if err != nil {
				return err
			}
			if state.enabled && !matchesTarget(kind, state, socksPort, httpPort) {
				return fmt.Errorf("%s already has %s enabled; disable it or configure your browser manually", service, kind.label)
			}
			if state.enabled && matchesTarget(kind, state, socksPort, httpPort) {
				state.enabled = false
			}
			previous[service][kind] = state
		}
	}

	for _, service := range services {
		if err := m.setProxy(service, proxyKinds[0], httpPort); err != nil {
			_ = m.restore(previous)
			return err
		}
		if err := m.setProxy(service, proxyKinds[1], httpPort); err != nil {
			_ = m.restore(previous)
			return err
		}
		if err := m.setProxy(service, proxyKinds[2], socksPort); err != nil {
			_ = m.restore(previous)
			return err
		}
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

	err := m.restore(m.previous)
	m.previous = nil
	m.enabled = false
	return err
}

func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.enabled
}

func (m *Manager) listNetworkServices() ([]string, error) {
	output, err := m.runNetworksetup("-listallnetworkservices")
	if err != nil {
		return nil, err
	}

	var services []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}

	return services, nil
}

func (m *Manager) getProxyState(service string, kind proxyKind) (proxyState, error) {
	output, err := m.runNetworksetup(kind.get, service)
	if err != nil {
		return proxyState{}, err
	}

	return parseProxyState(output)
}

func (m *Manager) setProxy(service string, kind proxyKind, port int) error {
	if _, err := m.runNetworksetup(kind.set, service, loopbackHost, strconv.Itoa(port)); err != nil {
		return err
	}
	_, err := m.runNetworksetup(kind.setState, service, "on")
	return err
}

func (m *Manager) restore(previous map[string]map[proxyKind]proxyState) error {
	var errs []string

	for service, states := range previous {
		for _, kind := range proxyKinds {
			state := states[kind]
			if state.server != "" && state.port > 0 {
				if _, err := m.runNetworksetup(kind.set, service, state.server, strconv.Itoa(state.port)); err != nil {
					errs = append(errs, err.Error())
					continue
				}
			}

			targetState := "off"
			if state.enabled {
				targetState = "on"
			}
			if _, err := m.runNetworksetup(kind.setState, service, targetState); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("restore system proxy settings: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (m *Manager) runNetworksetup(args ...string) (string, error) {
	output, err := m.runner.Run("networksetup", args...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return "", fmt.Errorf("networksetup %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("networksetup %s: %w: %s", strings.Join(args, " "), err, detail)
	}

	return string(output), nil
}

func parseProxyState(output string) (proxyState, error) {
	var state proxyState

	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch key {
		case "enabled":
			state.enabled = strings.EqualFold(value, "yes") || value == "1"
		case "server":
			state.server = value
		case "port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return proxyState{}, fmt.Errorf("invalid proxy port %q", value)
			}
			state.port = port
		}
	}

	return state, nil
}

func matchesTarget(kind proxyKind, state proxyState, socksPort, httpPort int) bool {
	targetPort := httpPort
	if kind.label == "SOCKS proxy" {
		targetPort = socksPort
	}

	return state.server == loopbackHost && state.port == targetPort
}
