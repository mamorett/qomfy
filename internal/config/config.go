package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config mirrors the qomfy configuration model. The JSON tags match cc.py's
// AppConfig plus the extended fields described in qomfyplan.md.
type Config struct {
	ServerURL    string  `json:"server_url"`
	WorkflowsDir string  `json:"workflows_dir,omitempty"`
	DownloadsDir string  `json:"downloads_dir,omitempty"`
	PollInterval float64 `json:"poll_interval,omitempty"`
	Timeout      float64 `json:"timeout,omitempty"`
}

const (
	defaultDownloadsDir = "downloads"
	defaultPollInterval = 2.0
	defaultTimeout      = 1800.0
)

// ExpandHome expands a leading "~" to the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func (c *Config) applyDefaults() {
	if c.WorkflowsDir == "" {
		c.WorkflowsDir = ExpandHome(filepath.Join("~", ".config", "qomfy", "workflows"))
	} else {
		c.WorkflowsDir = ExpandHome(c.WorkflowsDir)
	}
	if c.DownloadsDir == "" {
		c.DownloadsDir = defaultDownloadsDir
	} else {
		c.DownloadsDir = ExpandHome(c.DownloadsDir)
	}
	if c.PollInterval == 0 {
		c.PollInterval = defaultPollInterval
	}
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
}

// ResolvePath implements the config lookup order:
//  1. explicit CLI path, 2. QOMFY_CONFIG env, 3. $XDG_CONFIG_HOME/qomfy/config.json,
//  4. ~/.config/qomfy/config.json.
//
// The workflows/downloads override flags do not affect the config file location.
func ResolvePath(cliPath string) string {
	if cliPath != "" {
		return ExpandHome(cliPath)
	}
	if env := os.Getenv("QOMFY_CONFIG"); env != "" {
		return ExpandHome(env)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(ExpandHome(xdg), "qomfy", "config.json")
	}
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".config", "qomfy", "config.json")
	}
	return filepath.Join(".config", "qomfy", "config.json")
}

// Load reads, parses and fills defaults for the config at path. server_url is
// required; a missing file or empty server_url is an error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Config file not found at %s. Run: qomfy config init", path)
		}
		return nil, fmt.Errorf("Could not read config %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("Invalid JSON in %s: %w", path, err)
	}
	c.applyDefaults()
	if c.ServerURL == "" {
		return nil, fmt.Errorf("server_url is required in %s", path)
	}
	return &c, nil
}

// WorkflowsDirFrom computes the workflows directory resolution without requiring
// a loaded/validated Config (used by `config workflows` and short-name commands
// even when server_url is absent). It reads workflows_dir from the config file
// if present, else the default ~/.config/qomfy/workflows. The cliOverride, when
// non-empty, wins over everything.
func WorkflowsDirFrom(cliOverride, configPath string) string {
	if cliOverride != "" {
		return ExpandHome(cliOverride)
	}
	if configPath == "" {
		return DefaultWorkflowsDir()
	}
	if data, err := os.ReadFile(configPath); err == nil {
		var partial struct {
			WorkflowsDir string `json:"workflows_dir"`
		}
		if json.Unmarshal(data, &partial) == nil && partial.WorkflowsDir != "" {
			return ExpandHome(partial.WorkflowsDir)
		}
	}
	return DefaultWorkflowsDir()
}

// DefaultWorkflowsDir returns the default workflows directory.
func DefaultWorkflowsDir() string {
	return ExpandHome(filepath.Join("~", ".config", "qomfy", "workflows"))
}

// DefaultServerURL is the value used by `qomfy config init` when --server-url
// is not supplied, matching cc.py.
const DefaultServerURL = "http://localhost:8188/"

// NewForInit builds a config object for writing via `config init`. Only the
// server_url is serialized (other fields are omitted via omitempty), matching
// the cc.py AppConfig model.
func NewForInit(serverURL string) *Config {
	return &Config{ServerURL: serverURL}
}

// Write writes the config as indented JSON to path, creating parent dirs.
func (c *Config) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
