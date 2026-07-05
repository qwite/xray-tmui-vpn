package xray

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/xtls/xray-core/core"
	featurestats "github.com/xtls/xray-core/features/stats"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
)

type Client struct {
	mu        sync.Mutex
	instance  *core.Instance
	logDir    string
	accessLog string
	errorLog  string
}

type Snapshot struct {
	Version       string
	UplinkBytes   int64
	DownlinkBytes int64
	LogLines      []string
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Start(config RuntimeConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.instance != nil {
		return errors.New("xray is already running")
	}

	logDir, err := os.MkdirTemp("", "xray-tmui-vpn-*")
	if err != nil {
		return err
	}
	accessLog := filepath.Join(logDir, "access.log")
	errorLog := filepath.Join(logDir, "error.log")
	if err := os.WriteFile(accessLog, nil, 0600); err != nil {
		_ = os.RemoveAll(logDir)
		return err
	}
	if err := os.WriteFile(errorLog, nil, 0600); err != nil {
		_ = os.RemoveAll(logDir)
		return err
	}

	config.AccessLogPath = accessLog
	config.ErrorLogPath = errorLog

	configJSON, err := BuildConfigJSON(config)
	if err != nil {
		_ = os.RemoveAll(logDir)
		return err
	}

	coreConfig, err := serial.DecodeJSONConfig(bytes.NewReader(configJSON))
	if err != nil {
		_ = os.RemoveAll(logDir)
		return err
	}

	builtConfig, err := coreConfig.Build()
	if err != nil {
		_ = os.RemoveAll(logDir)
		return err
	}

	instance, err := core.New(builtConfig)
	if err != nil {
		_ = os.RemoveAll(logDir)
		return err
	}

	if err := instance.Start(); err != nil {
		instance.Close()
		_ = os.RemoveAll(logDir)
		return err
	}

	c.instance = instance
	c.logDir = logDir
	c.accessLog = accessLog
	c.errorLog = errorLog
	return nil
}

func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.instance == nil {
		return nil
	}

	c.instance.Close()
	c.instance = nil
	if c.logDir != "" {
		_ = os.RemoveAll(c.logDir)
	}
	c.logDir = ""
	c.accessLog = ""
	c.errorLog = ""
	return nil
}

func (c *Client) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.instance != nil
}

func (c *Client) Version() string {
	return Version()
}

func Version() string {
	return core.Version()
}

func (c *Client) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := Snapshot{Version: core.Version()}
	if c.instance == nil {
		return snapshot
	}

	if manager, ok := c.instance.GetFeature(featurestats.ManagerType()).(featurestats.Manager); ok {
		snapshot.UplinkBytes = counterValue(manager, "outbound>>>proxy>>>traffic>>>uplink")
		snapshot.DownlinkBytes = counterValue(manager, "outbound>>>proxy>>>traffic>>>downlink")
	}

	snapshot.LogLines = append(snapshot.LogLines, readLogLines(c.errorLog)...)
	snapshot.LogLines = append(snapshot.LogLines, readLogLines(c.accessLog)...)
	return snapshot
}

func counterValue(manager featurestats.Manager, name string) int64 {
	counter := manager.GetCounter(name)
	if counter == nil {
		return 0
	}
	return counter.Value()
}

func readLogLines(path string) []string {
	if path == "" {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	lines := bytes.Split(content, []byte{'\n'})
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			result = append(result, string(line))
		}
	}
	return result
}
