//go:build !darwin && !windows

package systemproxy

import "sync"

type Manager struct {
	mu      sync.Mutex
	enabled bool
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Enable(_, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = true
	return nil
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = false
	return nil
}

func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.enabled
}
