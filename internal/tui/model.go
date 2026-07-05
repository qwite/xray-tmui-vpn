package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/qwites/xray-tmui-vpn/internal/profile"
	"github.com/qwites/xray-tmui-vpn/internal/systemproxy"
	"github.com/qwites/xray-tmui-vpn/internal/xray"
)

type field int

const (
	fieldVLESSLink field = iota
	fieldAddress
	fieldPort
	fieldUUID
	fieldServerName
	fieldFingerprint
	fieldALPN
	fieldPublicKey
	fieldShortID
	fieldSpiderX
	fieldFlow
	fieldSOCKSPort
	fieldHTTPPort
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type Model struct {
	engine     *xray.Client
	proxy      *systemproxy.Manager
	inputs     []textinput.Model
	focusIndex int
	security   string
	running    bool
	busy       bool
	showConfig bool
	status     string
	lastConfig string
	activeName string
	hasProfile bool
	editing    bool
	snapshot   xray.Snapshot
	logs       []string
}

type startedMsg struct{}
type stoppedMsg struct {
	snapshot xray.Snapshot
}
type metricsMsg struct {
	snapshot xray.Snapshot
}
type exportedLogsMsg struct {
	path string
	logs []string
}

type errMsg struct {
	err error
}

func NewModel() Model {
	inputs := make([]textinput.Model, 13)
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].CharLimit = 256
		inputs[i].Width = 44
	}

	inputs[fieldVLESSLink].Placeholder = "vless://..."
	inputs[fieldVLESSLink].CharLimit = 2048

	inputs[fieldAddress].Placeholder = "vpn.example.com"
	inputs[fieldAddress].SetValue("127.0.0.1")

	inputs[fieldPort].Placeholder = "443"
	inputs[fieldPort].SetValue("443")

	inputs[fieldUUID].Placeholder = "00000000-0000-0000-0000-000000000000"

	inputs[fieldServerName].Placeholder = "server name / SNI"
	inputs[fieldFingerprint].Placeholder = "chrome / firefox / safari"
	inputs[fieldFingerprint].SetValue("chrome")
	inputs[fieldALPN].Placeholder = "h2,http/1.1"
	inputs[fieldPublicKey].Placeholder = "reality public key"
	inputs[fieldShortID].Placeholder = "reality short id"
	inputs[fieldSpiderX].Placeholder = "/"
	inputs[fieldSpiderX].SetValue("/")
	inputs[fieldFlow].Placeholder = "xtls-rprx-vision"
	inputs[fieldSOCKSPort].Placeholder = "10808"
	inputs[fieldSOCKSPort].SetValue("10808")
	inputs[fieldHTTPPort].Placeholder = "10809"
	inputs[fieldHTTPPort].SetValue("10809")

	inputs[fieldAddress].Focus()

	engine := xray.NewClient()
	model := Model{
		engine:     engine,
		proxy:      systemproxy.NewManager(),
		inputs:     inputs,
		focusIndex: int(fieldAddress),
		security:   "reality",
		status:     "Idle",
		snapshot:   xray.Snapshot{Version: engine.Version()},
	}

	savedProfile, ok, err := profile.Load()
	if err != nil {
		model.status = "Load profile: " + err.Error()
		return model
	}
	if ok {
		model.applyConfig(savedProfile.Config)
		model.activeName = savedProfile.Name
		model.hasProfile = true
		model.status = "Ready"
		model.refreshConfig()
	}

	return model
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Sequence(m.stopCmd(), tea.Quit)
		case "tab", "shift+tab", "up", "down":
			if m.inEditMode() {
				m.moveFocus(msg.String())
			}
			return m, nil
		case "f2":
			if m.inEditMode() && !m.busy && !m.running {
				m.security = nextSecurity(m.security)
				m.ensureVisibleFocus()
			}
			return m, nil
		case "f3":
			m.showConfig = !m.showConfig
			m.refreshConfig()
			return m, nil
		case "f4":
			if m.inEditMode() && !m.busy && !m.running {
				m.importLink()
			}
			return m, nil
		case "f5":
			if !m.inEditMode() {
				return m, m.exportLogsCmd()
			}
			return m, nil
		case "e":
			if !m.inEditMode() && !m.busy && !m.running {
				m.editing = true
				return m, nil
			}
		case "enter":
			if m.busy {
				return m, nil
			}
			if m.running {
				m.busy = true
				m.status = "Disconnecting..."
				return m, m.stopCmd()
			}

			config, err := m.runtimeConfig()
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.activeName = m.profileName(config)
			if err := m.saveProfile(config, m.activeName); err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.refreshConfig()
			m.busy = true
			m.status = "Connecting..."
			m.logs = appendLog(m.logs, "Connecting to "+m.activeName)
			return m, m.startCmd(config)
		}
	case startedMsg:
		m.busy = false
		m.running = true
		m.status = "Connected"
		m.logs = appendLog(m.logs, "Xray started")
		m.logs = appendLog(m.logs, "System proxy enabled")
		return m, m.metricsCmd()
	case stoppedMsg:
		m.busy = false
		m.running = false
		m.status = "Disconnected"
		m.snapshot = msg.snapshot
		m.snapshot.Version = valueOr(m.snapshot.Version, m.engine.Version())
		m.logs = mergeLogs(m.logs, msg.snapshot.LogLines)
		m.logs = appendLog(m.logs, "Disconnected")
		return m, nil
	case errMsg:
		m.busy = false
		m.status = msg.err.Error()
		m.logs = appendLog(m.logs, "Error: "+msg.err.Error())
		return m, nil
	case metricsMsg:
		m.snapshot = msg.snapshot
		m.logs = mergeLogs(m.logs, msg.snapshot.LogLines)
		if m.running {
			return m, m.metricsCmd()
		}
		return m, nil
	case exportedLogsMsg:
		m.logs = msg.logs
		m.status = "Exported logs to " + msg.path
		return m, nil
	}

	if !m.inEditMode() {
		return m, nil
	}

	var cmd tea.Cmd
	m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.inEditMode() {
		return m.editView()
	}

	return m.dashboardView()
}

func (m Model) editView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("xray-tmui-vpn"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Embedded xray-core VLESS client"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Status: %s\n", m.renderStatus()))
	b.WriteString(fmt.Sprintf("Security: %s\n\n", m.security))

	labels := []string{
		"VLESS link",
		"Server address",
		"Server port",
		"VLESS UUID",
		"Server name",
		"Fingerprint",
		"ALPN",
		"Reality public key",
		"Reality short id",
		"Reality spiderX",
		"Flow",
		"Local SOCKS port",
		"Local HTTP port",
	}

	for i, input := range m.inputs {
		if !m.visibleField(field(i)) {
			continue
		}

		b.WriteString(labelStyle.Render(labels[i]))
		b.WriteString("\n")
		b.WriteString(input.View())
		b.WriteString("\n\n")
	}

	action := "enter connect"
	if m.running {
		action = "enter disconnect"
	}

	b.WriteString(helpStyle.Render(fmt.Sprintf("%s | tab focus | f2 security | f3 config | f4 import | esc quit", action)))
	if m.showConfig {
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render("Generated config"))
		b.WriteString("\n")
		b.WriteString(m.lastConfig)
	}

	return b.String()
}

func (m Model) dashboardView() string {
	profiles := []string{m.activeName}
	if profiles[0] == "" {
		profiles[0] = "current-profile"
	}

	left := m.panel("Profiles", 28, m.profileLines(profiles))
	right := m.panel("Status", 34, []string{
		"Xray: " + m.xrayState(),
		"Version: " + valueOr(m.snapshot.Version, m.engine.Version()),
		"Active profile: " + valueOr(m.activeName, "current-profile"),
		"Status: " + m.status,
		"Uplink: " + formatBytes(m.snapshot.UplinkBytes),
		"Downlink: " + formatBytes(m.snapshot.DownlinkBytes),
		"SOCKS: 127.0.0.1:" + m.inputs[fieldSOCKSPort].Value(),
		"HTTP: 127.0.0.1:" + m.inputs[fieldHTTPPort].Value(),
	})

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	action := "enter connect"
	if m.running {
		action = "enter disconnect"
	}
	help := helpStyle.Render(fmt.Sprintf("%s | e edit | f3 config | f5 export logs | esc quit", action))
	if m.showConfig {
		return content + "\n\n" + help + "\n\n" + labelStyle.Render("Generated config") + "\n" + m.lastConfig
	}
	return content + "\n\n" + help
}

func (m Model) inEditMode() bool {
	return m.editing || !m.hasProfile
}

func (m Model) panel(title string, width int, lines []string) string {
	body := "-"
	if len(lines) > 0 {
		body = strings.Join(lines, "\n")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("245")).
		Padding(0, 1).
		Width(width).
		Render(labelStyle.Render(title) + "\n" + body)
}

func (m Model) profileLines(profiles []string) []string {
	lines := make([]string, 0, len(profiles))
	for i, profile := range profiles {
		prefix := "  "
		if i == 0 {
			prefix = "* "
		}
		lines = append(lines, prefix+profile)
	}
	return lines
}

func (m Model) xrayState() string {
	if m.busy {
		return strings.ToLower(strings.TrimSuffix(m.status, "..."))
	}
	if m.running {
		return "running"
	}
	return "stopped"
}

func (m Model) renderStatus() string {
	if m.running {
		return okStyle.Render(m.status)
	}
	if m.busy {
		return labelStyle.Render(m.status)
	}
	if m.status == "Idle" || m.status == "Disconnected" {
		return labelStyle.Render(m.status)
	}
	return errStyle.Render(m.status)
}

func (m *Model) moveFocus(key string) {
	m.inputs[m.focusIndex].Blur()

	for {
		switch key {
		case "shift+tab", "up":
			m.focusIndex--
		default:
			m.focusIndex++
		}

		if m.focusIndex >= len(m.inputs) {
			m.focusIndex = 0
		}
		if m.focusIndex < 0 {
			m.focusIndex = len(m.inputs) - 1
		}
		if m.visibleField(field(m.focusIndex)) {
			break
		}
	}

	m.inputs[m.focusIndex].Focus()
}

func (m *Model) ensureVisibleFocus() {
	if m.visibleField(field(m.focusIndex)) {
		return
	}

	m.inputs[m.focusIndex].Blur()
	for i := range m.inputs {
		if m.visibleField(field(i)) {
			m.focusIndex = i
			m.inputs[m.focusIndex].Focus()
			return
		}
	}
}

func (m Model) visibleField(candidate field) bool {
	if m.security == "none" && (candidate == fieldServerName || candidate == fieldFingerprint || candidate == fieldALPN) {
		return false
	}
	if m.security != "reality" && (candidate == fieldPublicKey || candidate == fieldShortID || candidate == fieldSpiderX) {
		return false
	}

	return true
}

func (m Model) startCmd(config xray.RuntimeConfig) tea.Cmd {
	return func() tea.Msg {
		if err := m.engine.Start(config); err != nil {
			return errMsg{err: err}
		}
		if err := m.proxy.Enable(config.SOCKSPort, config.HTTPPort); err != nil {
			_ = m.engine.Stop()
			return errMsg{err: err}
		}
		return startedMsg{}
	}
}

func (m Model) metricsCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return metricsMsg{snapshot: m.engine.Snapshot()}
	})
}

func (m Model) exportLogsCmd() tea.Cmd {
	existing := append([]string(nil), m.logs...)
	return func() tea.Msg {
		logs := mergeLogs(existing, m.engine.Snapshot().LogLines)
		path, err := profile.ExportLogs(logs)
		if err != nil {
			return errMsg{err: err}
		}
		return exportedLogsMsg{path: path, logs: logs}
	}
}

func (m Model) stopCmd() tea.Cmd {
	return func() tea.Msg {
		snapshot := m.engine.Snapshot()
		if err := m.proxy.Disable(); err != nil {
			_ = m.engine.Stop()
			return errMsg{err: err}
		}
		if err := m.engine.Stop(); err != nil {
			return errMsg{err: err}
		}
		return stoppedMsg{snapshot: snapshot}
	}
}

func (m *Model) refreshConfig() {
	config, err := m.runtimeConfig()
	if err != nil {
		m.lastConfig = err.Error()
		return
	}

	configJSON, err := xray.BuildConfigJSON(config)
	if err != nil {
		m.lastConfig = err.Error()
		return
	}

	m.lastConfig = string(configJSON)
}

func (m *Model) importLink() {
	config, name, err := xray.ParseVLESSLink(m.inputs[fieldVLESSLink].Value(), m.baseConfig())
	if err != nil {
		m.status = err.Error()
		return
	}

	m.applyConfig(config)
	m.refreshConfig()
	m.activeName = name
	if strings.TrimSpace(m.activeName) == "" {
		m.activeName = m.profileName(config)
	}
	if err := m.saveProfile(config, m.activeName); err != nil {
		m.status = err.Error()
		return
	}
	m.editing = false
	if name == "" {
		m.status = "Imported VLESS config"
		return
	}
	m.status = fmt.Sprintf("Imported %s", name)
}

func (m *Model) applyConfig(config xray.RuntimeConfig) {
	m.security = config.Security
	m.inputs[fieldAddress].SetValue(config.ServerAddress)
	m.inputs[fieldPort].SetValue(strconv.Itoa(config.ServerPort))
	m.inputs[fieldUUID].SetValue(config.UUID)
	m.inputs[fieldServerName].SetValue(config.ServerName)
	m.inputs[fieldFingerprint].SetValue(config.Fingerprint)
	m.inputs[fieldALPN].SetValue(strings.Join(config.ALPN, ","))
	m.inputs[fieldPublicKey].SetValue(config.PublicKey)
	m.inputs[fieldShortID].SetValue(config.ShortID)
	m.inputs[fieldSpiderX].SetValue(config.SpiderX)
	m.inputs[fieldFlow].SetValue(config.Flow)
	m.inputs[fieldSOCKSPort].SetValue(strconv.Itoa(config.SOCKSPort))
	m.inputs[fieldHTTPPort].SetValue(strconv.Itoa(config.HTTPPort))
	m.ensureVisibleFocus()
}

func (m Model) baseConfig() xray.RuntimeConfig {
	socksPort, err := parsePort(m.inputs[fieldSOCKSPort].Value(), "socks port")
	if err != nil {
		socksPort = 10808
	}

	httpPort, err := parsePort(m.inputs[fieldHTTPPort].Value(), "http port")
	if err != nil {
		httpPort = 10809
	}

	return xray.RuntimeConfig{
		SOCKSPort: socksPort,
		HTTPPort:  httpPort,
		SpiderX:   "/",
	}
}

func (m Model) runtimeConfig() (xray.RuntimeConfig, error) {
	serverPort, err := parsePort(m.inputs[fieldPort].Value(), "server port")
	if err != nil {
		return xray.RuntimeConfig{}, err
	}

	socksPort, err := parsePort(m.inputs[fieldSOCKSPort].Value(), "socks port")
	if err != nil {
		return xray.RuntimeConfig{}, err
	}

	httpPort, err := parsePort(m.inputs[fieldHTTPPort].Value(), "http port")
	if err != nil {
		return xray.RuntimeConfig{}, err
	}

	return xray.RuntimeConfig{
		ServerAddress: strings.TrimSpace(m.inputs[fieldAddress].Value()),
		ServerPort:    serverPort,
		UUID:          strings.TrimSpace(m.inputs[fieldUUID].Value()),
		ServerName:    strings.TrimSpace(m.inputs[fieldServerName].Value()),
		Security:      m.security,
		Fingerprint:   strings.TrimSpace(m.inputs[fieldFingerprint].Value()),
		ALPN:          splitListInput(m.inputs[fieldALPN].Value()),
		PublicKey:     strings.TrimSpace(m.inputs[fieldPublicKey].Value()),
		ShortID:       strings.TrimSpace(m.inputs[fieldShortID].Value()),
		SpiderX:       strings.TrimSpace(m.inputs[fieldSpiderX].Value()),
		Flow:          strings.TrimSpace(m.inputs[fieldFlow].Value()),
		SOCKSPort:     socksPort,
		HTTPPort:      httpPort,
	}, nil
}

func (m Model) profileName(config xray.RuntimeConfig) string {
	if strings.TrimSpace(m.activeName) != "" {
		return strings.TrimSpace(m.activeName)
	}
	return fmt.Sprintf("%s-%s", strings.TrimSpace(config.ServerAddress), config.Security)
}

func (m *Model) saveProfile(config xray.RuntimeConfig, name string) error {
	if err := profile.Save(profile.Profile{Name: name, Config: config}); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	m.hasProfile = true
	m.editing = false
	return nil
}

func appendLog(logs []string, line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return logs
	}
	return trimLogLines(append(logs, time.Now().Format("2006/01/02 15:04:05")+" "+line))
}

func mergeLogs(existing []string, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, line := range existing {
		seen[line] = struct{}{}
	}
	for _, line := range incoming {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		existing = append(existing, line)
		seen[line] = struct{}{}
	}
	return trimLogLines(existing)
}

func trimLogLines(lines []string) []string {
	const maxLogs = 200
	if len(lines) <= maxLogs {
		return lines
	}
	return lines[len(lines)-maxLogs:]
}

func lastLines(lines []string, count int) []string {
	if len(lines) == 0 {
		return []string{"Waiting for xray logs"}
	}
	if len(lines) <= count {
		return lines
	}
	return lines[len(lines)-count:]
}

func formatBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}

	div := int64(unit)
	exp := 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(value)/float64(div), "KMGTPE"[exp])
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func splitListInput(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

func parsePort(value string, label string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("%s must be between 1 and 65535", label)
	}

	return port, nil
}

func nextSecurity(current string) string {
	switch current {
	case "none":
		return "tls"
	case "tls":
		return "reality"
	default:
		return "none"
	}
}
