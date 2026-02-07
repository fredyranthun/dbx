package config

import (
	"fmt"
	"strings"
)

// Validate checks config structure and required values, failing fast.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config: must not be nil")
	}

	defaults := cfg.EffectiveDefaults()

	if len(defaults.PortRange) != 2 {
		return fmt.Errorf("defaults.port_range: expected exactly 2 values, got %d", len(defaults.PortRange))
	}
	if defaults.PortRange[0] >= defaults.PortRange[1] {
		return fmt.Errorf("defaults.port_range: expected min < max, got [%d,%d]", defaults.PortRange[0], defaults.PortRange[1])
	}
	if strings.TrimSpace(defaults.Bind) == "" {
		return fmt.Errorf("defaults.bind: must not be empty")
	}

	seenServices := make(map[string]struct{}, len(cfg.Services))
	for i := range cfg.Services {
		svc := cfg.Services[i]
		serviceName := strings.TrimSpace(svc.Name)
		if serviceName == "" {
			return fmt.Errorf("services[%d].name: must not be empty", i)
		}
		if _, exists := seenServices[serviceName]; exists {
			return fmt.Errorf("services[%s].name: duplicate service name", serviceName)
		}
		seenServices[serviceName] = struct{}{}

		for envName, envCfg := range svc.Envs {
			envKey := strings.TrimSpace(envName)
			if envKey == "" {
				return fmt.Errorf("services[%s].envs: env key must not be empty", serviceName)
			}
			path := fmt.Sprintf("services[%s].envs[%s]", serviceName, envKey)
			if strings.TrimSpace(envCfg.TargetInstanceID) == "" {
				return fmt.Errorf("%s.target_instance_id: must not be empty", path)
			}
			if strings.TrimSpace(envCfg.RemoteHost) == "" {
				return fmt.Errorf("%s.remote_host: must not be empty", path)
			}
			if envCfg.RemotePort < 1 || envCfg.RemotePort > 65535 {
				return fmt.Errorf("%s.remote_port: must be between 1 and 65535", path)
			}
			if envCfg.LocalPort < 0 || envCfg.LocalPort > 65535 {
				return fmt.Errorf("%s.local_port: must be between 1 and 65535", path)
			}
		}
	}

	return nil
}
