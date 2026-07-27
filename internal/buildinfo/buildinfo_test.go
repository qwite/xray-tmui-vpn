package buildinfo

import "testing"

func TestDisplayVersion(t *testing.T) {
	original := Version
	t.Cleanup(func() {
		Version = original
	})

	tests := []struct {
		version string
		want    string
	}{
		{version: "dev", want: "dev"},
		{version: "", want: "dev"},
		{version: "0.1.0-alpha", want: "v0.1.0-alpha"},
		{version: "v0.1.0-alpha", want: "v0.1.0-alpha"},
	}

	for _, tt := range tests {
		Version = tt.version
		if got := DisplayVersion(); got != tt.want {
			t.Fatalf("DisplayVersion() = %q, want %q", got, tt.want)
		}
	}
}

func TestString(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = originalVersion, originalCommit, originalDate
	})

	Version = "0.1.0-alpha"
	Commit = "abc123"
	Date = "2026-07-27T12:00:00Z"

	want := "xray-tmui-vpn v0.1.0-alpha (commit abc123, built 2026-07-27T12:00:00Z)"
	if got := String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
