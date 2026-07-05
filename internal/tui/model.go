package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/qwites/xray-tmui-vpn/internal/daemon"
	"github.com/qwites/xray-tmui-vpn/internal/profile"
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
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	labelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	panelTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	activeStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	busyStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	stoppedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	statusLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	statusValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	profileNameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	trafficUpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	trafficDownStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("43"))
	endpointStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
)

type Model struct {
	inputs       []textinput.Model
	focusIndex   int
	security     string
	running      bool
	busy         bool
	showConfig   bool
	configScroll int
	width        int
	height       int
	status       string
	statusTick   int
	lastConfig   string
	activeName   string
	hasProfile   bool
	editing      bool
	snapshot     xray.Snapshot
	logs         []string
}

type startedMsg struct {
	snapshot xray.Snapshot
}
type stoppedMsg struct {
	snapshot xray.Snapshot
}
type metricsMsg struct {
	snapshot xray.Snapshot
	running  bool
	busy     bool
	status   string
}
type animationTickMsg struct{}
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

	model := Model{
		inputs:     inputs,
		focusIndex: int(fieldAddress),
		security:   "reality",
		status:     "Idle",
		snapshot:   xray.Snapshot{Version: xray.Version()},
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

	state, active, err := daemon.Status()
	if err != nil {
		model.status = "Load daemon state: " + err.Error()
		return model
	}
	if state.Profile.Config.UUID != "" {
		model.applyConfig(state.Profile.Config)
		model.activeName = state.Profile.Name
		model.hasProfile = true
		model.refreshConfig()
	}
	if active {
		model.running = daemon.IsConnected(state)
		model.busy = daemon.IsConnecting(state) || daemon.IsDisconnecting(state)
		model.status = daemon.DisplayStatus(state, active)
		model.snapshot = daemon.SnapshotFromState(state)
		model.logs = state.LogLines
	} else if daemon.IsError(state) {
		model.status = daemon.DisplayStatus(state, active)
	}

	return model
}

func (m Model) Init() tea.Cmd {
	if m.running {
		return tea.Batch(textinput.Blink, m.metricsCmd())
	}
	if m.busy {
		return tea.Batch(textinput.Blink, m.metricsCmd(), m.animationCmd())
	}
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampConfigScroll()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.showConfig {
				m.showConfig = false
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.showConfig {
				m.scrollConfig(-1)
				return m, nil
			}
			if m.inEditMode() && msg.String() == "up" {
				m.moveFocus(msg.String())
			}
			return m, nil
		case "down", "j":
			if m.showConfig {
				m.scrollConfig(1)
				return m, nil
			}
			if m.inEditMode() && msg.String() == "down" {
				m.moveFocus(msg.String())
			}
			return m, nil
		case "pgup":
			if m.showConfig {
				m.scrollConfig(-m.configPageSize())
			}
			return m, nil
		case "pgdown":
			if m.showConfig {
				m.scrollConfig(m.configPageSize())
			}
			return m, nil
		case "home":
			if m.showConfig {
				m.configScroll = 0
			}
			return m, nil
		case "end":
			if m.showConfig {
				m.configScroll = m.maxConfigScroll()
			}
			return m, nil
		case "tab", "shift+tab":
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
			m.configScroll = 0
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
				m.statusTick = 0
				return m, tea.Batch(m.stopCmd(), m.animationCmd())
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
			m.statusTick = 0
			m.logs = appendLog(m.logs, "Connecting to "+m.activeName)
			return m, tea.Batch(m.startCmd(config), m.animationCmd())
		}
	case startedMsg:
		m.busy = false
		m.statusTick = 0
		m.running = true
		m.status = "Connected"
		m.snapshot = msg.snapshot
		m.logs = mergeLogs(m.logs, msg.snapshot.LogLines)
		m.logs = appendLog(m.logs, "Xray started")
		m.logs = appendLog(m.logs, "System proxy enabled")
		return m, m.metricsCmd()
	case stoppedMsg:
		m.busy = false
		m.statusTick = 0
		m.running = false
		m.status = "Disconnected"
		m.snapshot = msg.snapshot
		m.snapshot.Version = valueOr(m.snapshot.Version, xray.Version())
		m.logs = mergeLogs(m.logs, msg.snapshot.LogLines)
		m.logs = appendLog(m.logs, "Disconnected")
		return m, nil
	case errMsg:
		m.busy = false
		m.statusTick = 0
		m.status = msg.err.Error()
		m.logs = appendLog(m.logs, "Error: "+msg.err.Error())
		return m, nil
	case animationTickMsg:
		if !m.busy {
			return m, nil
		}
		m.statusTick++
		return m, m.animationCmd()
	case metricsMsg:
		m.snapshot = msg.snapshot
		m.logs = mergeLogs(m.logs, msg.snapshot.LogLines)
		m.running = msg.running
		m.busy = msg.busy
		if msg.status != "" {
			m.status = msg.status
		}
		if m.running || m.busy {
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
	if m.showConfig {
		return m.configView()
	}
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

	return b.String()
}

func (m Model) dashboardView() string {
	profiles := []string{m.activeName}
	if profiles[0] == "" {
		profiles[0] = "current-profile"
	}

	left := m.panel("Profiles", 28, m.profileLines(profiles))
	right := m.panel("Status", 38, m.statusLines())

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	action := "enter connect"
	if m.running {
		action = "enter disconnect"
	}
	help := helpStyle.Render(fmt.Sprintf("%s | e edit | f3 config | f5 export logs | esc quit", action))
	return content + "\n\n" + help
}

func (m Model) configView() string {
	lines := strings.Split(m.lastConfig, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	pageSize := m.configPageSize()
	start := minInt(m.configScroll, len(lines))
	end := minInt(start+pageSize, len(lines))
	visible := lines[start:end]

	header := titleStyle.Render("Generated config")
	help := helpStyle.Render("f3/esc dashboard | up/down scroll | pgup/pgdown page | home/end jump")
	position := helpStyle.Render(fmt.Sprintf("lines %d-%d of %d", start+1, maxInt(start+1, end), len(lines)))
	width := m.width
	if width <= 0 {
		width = 96
	}
	if width > 4 {
		width -= 4
	}

	body := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("245")).
		Padding(0, 1).
		Width(width).
		Height(pageSize).
		Render(strings.Join(visible, "\n"))

	return strings.Join([]string{header, help, position, body}, "\n")
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
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Width(width).
		Render(panelTitleStyle.Render(title) + "\n" + body)
}

func (m Model) profileLines(profiles []string) []string {
	lines := make([]string, 0, len(profiles))
	for i, profile := range profiles {
		prefix := "  "
		if i == 0 {
			prefix = "* "
			lines = append(lines, activeStyle.Render(prefix)+profileNameStyle.Render(profile))
			continue
		}
		lines = append(lines, stoppedStyle.Render(prefix+profile))
	}
	return lines
}

func (m Model) statusLines() []string {
	return []string{
		statusLine("Xray", m.styledXrayState()),
		statusLine("Version", statusValueStyle.Render(valueOr(m.snapshot.Version, xray.Version()))),
		statusLine("Active profile", profileNameStyle.Render(valueOr(m.activeName, "current-profile"))),
		statusLine("Status", m.styledStatus()),
		statusLine("Uplink", trafficUpStyle.Render(formatBytes(m.snapshot.UplinkBytes))),
		statusLine("Downlink", trafficDownStyle.Render(formatBytes(m.snapshot.DownlinkBytes))),
		statusLine("SOCKS", endpointStyle.Render("127.0.0.1:"+m.inputs[fieldSOCKSPort].Value())),
		statusLine("HTTP", endpointStyle.Render("127.0.0.1:"+m.inputs[fieldHTTPPort].Value())),
	}
}

func statusLine(label string, value string) string {
	return statusLabelStyle.Render(label+": ") + value
}

func (m Model) styledXrayState() string {
	state := m.xrayState()
	if m.busy {
		return busyStyle.Render(state)
	}
	if m.running {
		return okStyle.Bold(true).Render(state)
	}
	return stoppedStyle.Render(state)
}

func (m Model) styledStatus() string {
	if m.busy {
		return busyStyle.Render(m.renderStatusText())
	}
	if m.running {
		return okStyle.Bold(true).Render(m.renderStatusText())
	}
	if m.status == "Ready" || m.status == "Idle" || m.status == "Disconnected" {
		return stoppedStyle.Render(m.status)
	}
	return errStyle.Render(m.status)
}

func (m Model) xrayState() string {
	if m.busy {
		return strings.ToLower(strings.TrimSuffix(m.renderStatusText(), "..."))
	}
	if m.running {
		return "running"
	}
	return "stopped"
}

func (m Model) renderStatus() string {
	if m.running {
		return okStyle.Render(m.renderStatusText())
	}
	if m.busy {
		return labelStyle.Render(m.renderStatusText())
	}
	if m.status == "Idle" || m.status == "Disconnected" {
		return labelStyle.Render(m.status)
	}
	return errStyle.Render(m.status)
}

func (m Model) renderStatusText() string {
	if !m.busy {
		return m.status
	}

	base := strings.TrimRight(m.status, ".")
	frames := []string{"   ", ".  ", ".. ", "..."}
	return base + frames[m.statusTick%len(frames)]
}

func (m *Model) scrollConfig(delta int) {
	m.configScroll += delta
	m.clampConfigScroll()
}

func (m *Model) clampConfigScroll() {
	if m.configScroll < 0 {
		m.configScroll = 0
	}
	maxScroll := m.maxConfigScroll()
	if m.configScroll > maxScroll {
		m.configScroll = maxScroll
	}
}

func (m Model) maxConfigScroll() int {
	lines := strings.Split(m.lastConfig, "\n")
	return maxInt(0, len(lines)-m.configPageSize())
}

func (m Model) configPageSize() int {
	if m.height <= 0 {
		return 24
	}
	return maxInt(6, m.height-7)
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
		state, err := daemon.Start(profile.Profile{Name: m.activeName, Config: config})
		if err != nil {
			return errMsg{err: err}
		}
		return startedMsg{snapshot: daemon.SnapshotFromState(state)}
	}
}

func (m Model) metricsCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		state, running, err := daemon.Status()
		if err != nil {
			return errMsg{err: err}
		}
		busy := running && (daemon.IsConnecting(state) || daemon.IsDisconnecting(state))
		connected := running && daemon.IsConnected(state)
		status := daemon.DisplayStatus(state, running)
		return metricsMsg{snapshot: daemon.SnapshotFromState(state), running: connected, busy: busy, status: status}
	})
}

func (m Model) animationCmd() tea.Cmd {
	return tea.Tick(180*time.Millisecond, func(time.Time) tea.Msg {
		return animationTickMsg{}
	})
}

func (m Model) exportLogsCmd() tea.Cmd {
	existing := append([]string(nil), m.logs...)
	return func() tea.Msg {
		state, _, err := daemon.Status()
		if err != nil {
			return errMsg{err: err}
		}
		logs := mergeLogs(existing, state.LogLines)
		path, err := profile.ExportLogs(logs)
		if err != nil {
			return errMsg{err: err}
		}
		return exportedLogsMsg{path: path, logs: logs}
	}
}

func (m Model) stopCmd() tea.Cmd {
	return func() tea.Msg {
		state, err := daemon.Stop()
		if err != nil {
			return errMsg{err: err}
		}
		return stoppedMsg{snapshot: daemon.SnapshotFromState(state)}
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
