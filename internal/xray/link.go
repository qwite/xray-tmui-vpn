package xray

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func ParseVLESSLink(raw string, base RuntimeConfig) (RuntimeConfig, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RuntimeConfig{}, "", errors.New("vless link is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return RuntimeConfig{}, "", err
	}
	if parsed.Scheme != "vless" {
		return RuntimeConfig{}, "", fmt.Errorf("unsupported link scheme %q", parsed.Scheme)
	}
	if parsed.User == nil || strings.TrimSpace(parsed.User.Username()) == "" {
		return RuntimeConfig{}, "", errors.New("vless link is missing uuid")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return RuntimeConfig{}, "", errors.New("vless link is missing server address")
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 || port > 65535 {
		return RuntimeConfig{}, "", errors.New("vless link is missing a valid server port")
	}

	query := parsed.Query()
	if encryption := query.Get("encryption"); encryption != "" && encryption != "none" {
		return RuntimeConfig{}, "", fmt.Errorf("unsupported vless encryption %q", encryption)
	}
	if network := query.Get("type"); network != "" && network != "tcp" {
		return RuntimeConfig{}, "", fmt.Errorf("unsupported vless network type %q", network)
	}

	security := query.Get("security")
	if security == "" {
		security = "none"
	}

	config := base
	config.ServerAddress = parsed.Hostname()
	config.ServerPort = port
	config.UUID = parsed.User.Username()
	config.Security = security
	config.ServerName = firstQuery(query, "sni", "peer")
	config.Fingerprint = query.Get("fp")
	config.ALPN = splitList(query.Get("alpn"))
	config.Flow = query.Get("flow")

	switch security {
	case "none", "tls":
	case "reality":
		config.PublicKey = query.Get("pbk")
		config.ShortID = query.Get("sid")
		config.SpiderX = query.Get("spx")
		if config.SpiderX == "" {
			config.SpiderX = "/"
		}
	default:
		return RuntimeConfig{}, "", fmt.Errorf("unsupported security mode %q", security)
	}

	if err := config.Validate(); err != nil {
		return RuntimeConfig{}, "", err
	}

	return config, parsed.Fragment, nil
}

func firstQuery(query url.Values, keys ...string) string {
	for _, key := range keys {
		if value := query.Get(key); value != "" {
			return value
		}
	}

	return ""
}

func splitList(value string) []string {
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
