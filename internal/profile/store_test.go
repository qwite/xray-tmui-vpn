package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qwites/xray-tmui-vpn/internal/xray"
)

func TestSaveLoadProfile(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())

	want := Profile{
		Name: "example-reality-profile",
		Config: xray.RuntimeConfig{
			ServerAddress: "vpn.example.com",
			ServerPort:    443,
			UUID:          "11111111-1111-4111-8111-111111111111",
			ServerName:    "vpn.example.com",
			Security:      "reality",
			Fingerprint:   "chrome",
			PublicKey:     "example-public-key",
			SpiderX:       "/",
			SOCKSPort:     10808,
			HTTPPort:      10809,
			AccessLogPath: "/tmp/access.log",
			ErrorLogPath:  "/tmp/error.log",
		},
	}

	if err := Save(want); err != nil {
		t.Fatal(err)
	}

	got, ok, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("profile was not found")
	}
	if got.Name != want.Name {
		t.Fatalf("name = %q", got.Name)
	}
	if got.Config.ServerAddress != want.Config.ServerAddress {
		t.Fatalf("server address = %q", got.Config.ServerAddress)
	}
	if got.Config.AccessLogPath != "" || got.Config.ErrorLogPath != "" {
		t.Fatalf("transient log paths were persisted: %#v", got.Config)
	}
}

func TestExportLogs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)

	path, err := ExportLogs([]string{"line one", "line two"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("log dir = %q, want %q", filepath.Dir(path), dir)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "line one\nline two\n" {
		t.Fatalf("exported logs = %q", got)
	}
}
