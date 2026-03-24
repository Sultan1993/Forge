package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port int
}

func Load() (*Config, error) {
	cfg := &Config{
		Port: 8080,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(filepath.Join(home, ".forge", "config.yaml"))
	if err != nil {
		return cfg, nil // no config file is fine, use defaults
	}

	// Minimal YAML-like parser for "port: 8080" — avoids a YAML dependency
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "port" {
			p, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid port value: %s", val)
			}
			if p < 1 || p > 65535 {
				return nil, fmt.Errorf("port out of range: %d", p)
			}
			cfg.Port = p
		}
	}

	return cfg, nil
}
