package main

import "testing"

func TestVersionRequested(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: []string{"--version"}, want: true},
		{args: []string{"version"}, want: true},
		{args: nil, want: false},
		{args: []string{"daemon"}, want: false},
		{args: []string{"--version", "extra"}, want: false},
	}

	for _, tt := range tests {
		if got := versionRequested(tt.args); got != tt.want {
			t.Fatalf("versionRequested(%q) = %t, want %t", tt.args, got, tt.want)
		}
	}
}
