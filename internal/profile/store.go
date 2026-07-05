package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qwites/xray-tmui-vpn/internal/xray"
)

const envConfigDir = "XRAY_TMUI_VPN_CONFIG_DIR"

type Profile struct {
	Name   string             `json:"name"`
	Config xray.RuntimeConfig `json:"config"`
}

func Load() (Profile, bool, error) {
	path, err := profilePath()
	if err != nil {
		return Profile{}, false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Profile{}, false, nil
		}
		return Profile{}, false, err
	}

	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, false, err
	}

	profile.Config.AccessLogPath = ""
	profile.Config.ErrorLogPath = ""
	if err := profile.Config.Validate(); err != nil {
		return Profile{}, false, err
	}
	if strings.TrimSpace(profile.Name) == "" {
		profile.Name = defaultName(profile.Config)
	}

	return profile, true, nil
}

func Save(profile Profile) error {
	profile.Config.AccessLogPath = ""
	profile.Config.ErrorLogPath = ""
	if err := profile.Config.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(profile.Name) == "" {
		profile.Name = defaultName(profile.Config)
	}

	path, err := profilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0600)
}

func ExportLogs(lines []string) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "xray-tmui-vpn.log")
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return path, os.WriteFile(path, []byte(content), 0600)
}

func profilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profile.json"), nil
}

func ConfigDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(envConfigDir)); dir != "" {
		return dir, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xray-tmui-vpn"), nil
}

func DataDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(envConfigDir)); dir != "" {
		return dir, nil
	}

	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xray-tmui-vpn"), nil
}

func defaultName(config xray.RuntimeConfig) string {
	return fmt.Sprintf("%s-%s", strings.TrimSpace(config.ServerAddress), config.Security)
}
