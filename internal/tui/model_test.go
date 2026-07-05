package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestEditShortcutDoesNotBlockTypingEInEditMode(t *testing.T) {
	model := newTestModel(t)
	model.inputs[fieldAddress].SetValue("")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(Model)

	if got := model.inputs[fieldAddress].Value(); got != "e" {
		t.Fatalf("focused input value = %q, want e", got)
	}
}
