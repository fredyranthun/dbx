package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const configPathEnvVar = "DBX_CONFIG"

var defaultConfigNames = []string{"config.yml", "config.yaml", "config.json"}

// LoadConfig resolves and loads dbx config from YAML/JSON.
func LoadConfig(pathOverride string) (*Config, string, error) {
	configPath, err := resolveConfigPath(pathOverride)
	if err != nil {
		return nil, "", err
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, "", fmt.Errorf("read config %q: %w", configPath, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, "", fmt.Errorf("parse config %q: %w", configPath, err)
	}

	return &cfg, configPath, nil
}

func resolveConfigPath(pathOverride string) (string, error) {
	override := strings.TrimSpace(pathOverride)
	if override != "" {
		path, err := ensureConfigPathExists(override)
		if err != nil {
			return "", fmt.Errorf("config file from --config not found: %w", err)
		}
		return path, nil
	}

	envPath := strings.TrimSpace(os.Getenv(configPathEnvVar))
	if envPath != "" {
		path, err := ensureConfigPathExists(envPath)
		if err != nil {
			return "", fmt.Errorf("config file from %s not found: %w", configPathEnvVar, err)
		}
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	defaultDir := filepath.Join(homeDir, ".dbx")
	checkedPaths := make([]string, 0, len(defaultConfigNames))
	for _, name := range defaultConfigNames {
		candidate := filepath.Join(defaultDir, name)
		checkedPaths = append(checkedPaths, candidate)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("config file not found; checked: %s", strings.Join(checkedPaths, ", "))
}

func ensureConfigPathExists(path string) (string, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", path)
	}
	return path, nil
}
