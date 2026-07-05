package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/qwites/xray-tmui-vpn/internal/daemon"
	"github.com/qwites/xray-tmui-vpn/internal/profile"
	"github.com/qwites/xray-tmui-vpn/internal/xray"
)

const (
	testVLESSUUID = "11111111-1111-4111-8111-111111111111"
	testVLESSHost = "vpn.example.com"
	testVLESSName = "example-tls-profile"
	testVLESSLink = "vless://" + testVLESSUUID + "@" + testVLESSHost + ":443?encryption=none&type=tcp&security=tls&flow=xtls-rprx-vision&headerType=none&fp=firefox&sni=" + testVLESSHost + "&alpn=http%2F1.1#" + testVLESSName
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("XRAY_TMUI_VPN_CONFIG_DIR", t.TempDir())
	return NewModel()
}

func TestPlainLettersAreTypedIntoFocusedInput(t *testing.T) {
	model := newTestModel(t)
	model.inputs[fieldAddress].SetValue("")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)

	if got := model.inputs[fieldAddress].Value(); got != "sc" {
		t.Fatalf("focused input value = %q, want %q", got, "sc")
	}
	if model.security != "reality" {
		t.Fatalf("security = %q, want unchanged reality", model.security)
	}
	if model.showConfig {
		t.Fatal("showConfig changed after typing plain c")
	}
}

func TestFunctionKeysControlGlobalActions(t *testing.T) {
	model := newTestModel(t)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF2})
	model = updated.(Model)

	if model.security != "none" {
		t.Fatalf("security after f2 = %q, want none", model.security)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyF3})
	model = updated.(Model)

	if !model.showConfig {
		t.Fatal("showConfig was not toggled by f3")
	}
}

func TestImportVLESSLinkPopulatesFields(t *testing.T) {
	model := newTestModel(t)
	model.inputs[fieldVLESSLink].SetValue(testVLESSLink)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF4})
	model = updated.(Model)

	if model.security != "tls" {
		t.Fatalf("security = %q, want tls", model.security)
	}
	if got := model.inputs[fieldAddress].Value(); got != testVLESSHost {
		t.Fatalf("server address = %q", got)
	}
	if got := model.inputs[fieldUUID].Value(); got != testVLESSUUID {
		t.Fatalf("uuid = %q", got)
	}
	if got := model.inputs[fieldFlow].Value(); got != "xtls-rprx-vision" {
		t.Fatalf("flow = %q", got)
	}
	if got := model.inputs[fieldFingerprint].Value(); got != "firefox" {
		t.Fatalf("fingerprint = %q", got)
	}
	if got := model.inputs[fieldALPN].Value(); got != "http/1.1" {
		t.Fatalf("alpn = %q", got)
	}
	if got := model.status; got != "Imported "+testVLESSName {
		t.Fatalf("status = %q", got)
	}
	if !model.hasProfile {
		t.Fatal("profile was not marked saved")
	}
	if model.inEditMode() {
		t.Fatal("model stayed in edit mode after importing a profile")
	}
}

func TestSavedProfileLoadsIntoDashboard(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XRAY_TMUI_VPN_CONFIG_DIR", dir)

	model := NewModel()
	model.inputs[fieldVLESSLink].SetValue(testVLESSLink)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF4})
	model = updated.(Model)

	reloaded := NewModel()
	if !reloaded.hasProfile {
		t.Fatal("saved profile was not loaded")
	}
	if reloaded.inEditMode() {
		t.Fatal("loaded profile should open dashboard mode")
	}
	if got := reloaded.inputs[fieldAddress].Value(); got != testVLESSHost {
		t.Fatalf("server address = %q", got)
	}
}

func TestDaemonConnectingStateLoadsAsBusy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XRAY_TMUI_VPN_CONFIG_DIR", dir)

	state := daemon.State{
		PID:     os.Getpid(),
		Status:  "connecting",
		Version: xray.Version(),
		Profile: profile.Profile{
			Name: testVLESSName,
			Config: xray.RuntimeConfig{
				ServerAddress: testVLESSHost,
				ServerPort:    443,
				UUID:          testVLESSUUID,
				ServerName:    testVLESSHost,
				Security:      "tls",
				SOCKSPort:     10808,
				HTTPPort:      10809,
			},
		},
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	model := NewModel()
	if !model.busy {
		t.Fatal("connecting daemon state should load as busy")
	}
	if model.running {
		t.Fatal("connecting daemon state should not load as running")
	}
	if got := model.status; got != "Connecting..." {
		t.Fatalf("status = %q", got)
	}
}

func TestDashboardProfilesOnlyShowsActiveProfile(t *testing.T) {
	model := newTestModel(t)
	model.hasProfile = true
	model.activeName = testVLESSName

	view := model.dashboardView()
	if !strings.Contains(view, testVLESSName) {
		t.Fatalf("dashboard does not include active profile: %q", view)
	}
	if strings.Contains(view, "local-socks") || strings.Contains(view, "local-http") {
		t.Fatalf("dashboard includes proxy entries as profiles: %q", view)
	}
}

func TestConfigViewCanReturnToDashboardWithEsc(t *testing.T) {
	model := newTestModel(t)
	model.hasProfile = true
	model.activeName = testVLESSName
	model.lastConfig = "{\n  \"log\": {}\n}"

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF3})
	model = updated.(Model)

	if !model.showConfig {
		t.Fatal("config view was not opened")
	}
	if view := model.View(); !strings.Contains(view, "f3/esc dashboard") {
		t.Fatalf("config view does not show return controls: %q", view)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if model.showConfig {
		t.Fatal("config view did not close on esc")
	}
	if view := model.View(); !strings.Contains(view, testVLESSName) {
		t.Fatalf("dashboard was not restored: %q", view)
	}
}

func TestConfigViewScrolls(t *testing.T) {
	model := newTestModel(t)
	model.hasProfile = true
	model.activeName = testVLESSName
	model.height = 14
	model.lastConfig = strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
	}, "\n")
	model.showConfig = true

	if strings.Contains(model.configView(), "line 8") {
		t.Fatal("last line is visible before scrolling")
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(Model)

	if model.configScroll != 1 {
		t.Fatalf("configScroll = %d, want 1", model.configScroll)
	}
	if !strings.Contains(model.configView(), "line 8") {
		t.Fatal("last line is not visible after scrolling")
	}
}

func TestBusyStatusTextAnimates(t *testing.T) {
	model := newTestModel(t)
	model.busy = true
	model.status = "Connecting..."

	if got := model.renderStatusText(); got != "Connecting   " {
		t.Fatalf("status frame 0 = %q", got)
	}

	updated, _ := model.Update(animationTickMsg{})
	model = updated.(Model)

	if got := model.renderStatusText(); got != "Connecting.  " {
		t.Fatalf("status frame 1 = %q", got)
	}
}

func TestAnimationTickStopsWhenNotBusy(t *testing.T) {
	model := newTestModel(t)
	model.busy = false
	model.statusTick = 3

	updated, cmd := model.Update(animationTickMsg{})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("animation tick scheduled another command while not busy")
	}
	if model.statusTick != 3 {
		t.Fatalf("statusTick = %d, want unchanged", model.statusTick)
	}
}

func TestEditShortcutDoesNotBlockTypingEInEditMode(t *testing.T) {
	model := newTestModel(t)
	model.inputs[fieldAddress].SetValue("")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(Model)

	if got := model.inputs[fieldAddress].Value(); got != "e" {
		t.Fatalf("focused input value = %q, want e", got)
	}
}
