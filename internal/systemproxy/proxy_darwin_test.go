package systemproxy

import "testing"

func TestParseProxyState(t *testing.T) {
	state, err := parseProxyState(`Enabled: Yes
Server: 127.0.0.1
Port: 10809
Authenticated Proxy Enabled: 0
`)
	if err != nil {
		t.Fatal(err)
	}

	if !state.enabled {
		t.Fatal("enabled = false, want true")
	}
	if state.server != "127.0.0.1" {
		t.Fatalf("server = %q", state.server)
	}
	if state.port != 10809 {
		t.Fatalf("port = %d", state.port)
	}
}

func TestParseProxyStateInvalidPort(t *testing.T) {
	_, err := parseProxyState(`Enabled: No
Server:
Port: nope
`)
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}
