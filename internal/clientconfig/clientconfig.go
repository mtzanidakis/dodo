package clientconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ClientConfig struct {
	URL      string `json:"url"`
	Token    string `json:"token"`
	LogLevel string `json:"log_level"`
}

type Flags struct {
	ConfigPath string
	URL        string
	Token      string
}

func Load(flags Flags) (ClientConfig, error) {
	cfg := ClientConfig{LogLevel: "info"}

	path := flags.ConfigPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			path = ".dodo/config.json"
		} else {
			path = filepath.Join(home, ".config", "dodo", "config.json")
		}
	}

	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return ClientConfig{}, fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ClientConfig{}, fmt.Errorf("reading %s: %w", path, err)
	}

	if flags.URL != "" {
		cfg.URL = flags.URL
	}
	if flags.Token != "" {
		cfg.Token = flags.Token
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	return cfg, nil
}

func Write(cfg ClientConfig, path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".config", "dodo", "config.json")
		} else {
			path = "dodo-config.json"
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	cfg.LogLevel = strings.TrimSpace(cfg.LogLevel)
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
