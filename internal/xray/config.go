package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type RuntimeConfig struct {
	ServerAddress string
	ServerPort    int
	UUID          string
	ServerName    string
	Security      string
	Fingerprint   string
	ALPN          []string
	PublicKey     string
	ShortID       string
	SpiderX       string
	Flow          string
	SOCKSPort     int
	HTTPPort      int
	AccessLogPath string
	ErrorLogPath  string
}

func (c RuntimeConfig) Validate() error {
	if strings.TrimSpace(c.ServerAddress) == "" {
		return errors.New("server address is required")
	}
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return errors.New("server port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.UUID) == "" {
		return errors.New("vless uuid is required")
	}
	if c.SOCKSPort <= 0 || c.SOCKSPort > 65535 {
		return errors.New("socks port must be between 1 and 65535")
	}
	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		return errors.New("http port must be between 1 and 65535")
	}

	switch c.Security {
	case "none", "tls":
	case "reality":
		if strings.TrimSpace(c.ServerName) == "" {
			return errors.New("server name is required for reality")
		}
		if strings.TrimSpace(c.PublicKey) == "" {
			return errors.New("public key is required for reality")
		}
	default:
		return fmt.Errorf("unsupported security mode %q", c.Security)
	}

	return nil
}

func BuildConfigJSON(c RuntimeConfig) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	user := map[string]any{
		"id":         strings.TrimSpace(c.UUID),
		"encryption": "none",
	}
	if strings.TrimSpace(c.Flow) != "" {
		user["flow"] = strings.TrimSpace(c.Flow)
	}

	streamSettings := map[string]any{
		"network":  "tcp",
		"security": c.Security,
	}

	switch c.Security {
	case "tls":
		tlsSettings := map[string]any{
			"serverName": strings.TrimSpace(c.ServerName),
		}
		if strings.TrimSpace(c.Fingerprint) != "" {
			tlsSettings["fingerprint"] = strings.TrimSpace(c.Fingerprint)
		}
		if len(c.ALPN) > 0 {
			tlsSettings["alpn"] = c.ALPN
		}
		streamSettings["tlsSettings"] = tlsSettings
	case "reality":
		fingerprint := strings.TrimSpace(c.Fingerprint)
		if fingerprint == "" {
			fingerprint = "chrome"
		}

		realitySettings := map[string]any{
			"serverName":  strings.TrimSpace(c.ServerName),
			"fingerprint": fingerprint,
			"publicKey":   strings.TrimSpace(c.PublicKey),
			"shortId":     strings.TrimSpace(c.ShortID),
			"spiderX":     strings.TrimSpace(c.SpiderX),
		}
		if len(c.ALPN) > 0 {
			realitySettings["alpn"] = c.ALPN
		}
		streamSettings["realitySettings"] = realitySettings
	}

	config := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"stats": map[string]any{},
		"policy": map[string]any{
			"system": map[string]any{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"inbounds": []map[string]any{
			{
				"tag":      "socks-in",
				"listen":   "127.0.0.1",
				"port":     c.SOCKSPort,
				"protocol": "socks",
				"settings": map[string]any{
					"auth": "noauth",
					"udp":  true,
				},
			},
			{
				"tag":      "http-in",
				"listen":   "127.0.0.1",
				"port":     c.HTTPPort,
				"protocol": "http",
			},
		},
		"outbounds": []map[string]any{
			{
				"tag":      "proxy",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []map[string]any{
						{
							"address": strings.TrimSpace(c.ServerAddress),
							"port":    c.ServerPort,
							"users":   []map[string]any{user},
						},
					},
				},
				"streamSettings": streamSettings,
			},
			{
				"tag":      "direct",
				"protocol": "freedom",
			},
			{
				"tag":      "block",
				"protocol": "blackhole",
			},
		},
	}

	logConfig := config["log"].(map[string]any)
	if strings.TrimSpace(c.AccessLogPath) != "" {
		logConfig["access"] = strings.TrimSpace(c.AccessLogPath)
	}
	if strings.TrimSpace(c.ErrorLogPath) != "" {
		logConfig["error"] = strings.TrimSpace(c.ErrorLogPath)
	}

	return json.MarshalIndent(config, "", "  ")
}
