//go:build windows

package systemproxy

import "testing"

func TestFormatProxyServer(t *testing.T) {
	got := formatProxyServer(10808, 10809)
	want := "http=127.0.0.1:10809;https=127.0.0.1:10809;socks=127.0.0.1:10808"
	if got != want {
		t.Fatalf("formatProxyServer() = %q, want %q", got, want)
	}
}
