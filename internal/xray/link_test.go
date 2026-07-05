package xray

import "testing"

const (
	testVLESSUUID = "11111111-1111-4111-8111-111111111111"
	testVLESSHost = "vpn.example.com"
	testVLESSName = "example-tls-profile"
	testVLESSLink = "vless://" + testVLESSUUID + "@" + testVLESSHost + ":443?encryption=none&type=tcp&security=tls&flow=xtls-rprx-vision&headerType=none&fp=firefox&sni=" + testVLESSHost + "&alpn=http%2F1.1#" + testVLESSName
)

func TestParseVLESSLinkTLS(t *testing.T) {
	config, name, err := ParseVLESSLink(testVLESSLink, RuntimeConfig{SOCKSPort: 10808, HTTPPort: 10809})
	if err != nil {
		t.Fatal(err)
	}

	if name != testVLESSName {
		t.Fatalf("name = %q, want decoded fragment", name)
	}
	if config.ServerAddress != testVLESSHost {
		t.Fatalf("server address = %q", config.ServerAddress)
	}
	if config.ServerPort != 443 {
		t.Fatalf("server port = %d", config.ServerPort)
	}
	if config.UUID != testVLESSUUID {
		t.Fatalf("uuid = %q", config.UUID)
	}
	if config.Security != "tls" {
		t.Fatalf("security = %q", config.Security)
	}
	if config.Flow != "xtls-rprx-vision" {
		t.Fatalf("flow = %q", config.Flow)
	}
	if config.Fingerprint != "firefox" {
		t.Fatalf("fingerprint = %q", config.Fingerprint)
	}
	if len(config.ALPN) != 1 || config.ALPN[0] != "http/1.1" {
		t.Fatalf("alpn = %#v", config.ALPN)
	}
}
