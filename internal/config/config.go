package config

// Config is the root dbx configuration model.
type Config struct {
	Defaults Defaults  `mapstructure:"defaults" json:"defaults" yaml:"defaults"`
	Services []Service `mapstructure:"services" json:"services" yaml:"services"`
}

// Defaults contains global settings used by session definitions.
type Defaults struct {
	Region                string `mapstructure:"region" json:"region" yaml:"region"`
	Profile               string `mapstructure:"profile" json:"profile" yaml:"profile"`
	Bind                  string `mapstructure:"bind" json:"bind" yaml:"bind"`
	PortRange             []int  `mapstructure:"port_range" json:"port_range" yaml:"port_range"`
	StartupTimeoutSeconds int    `mapstructure:"startup_timeout_seconds" json:"startup_timeout_seconds" yaml:"startup_timeout_seconds"`
	StopTimeoutSeconds    int    `mapstructure:"stop_timeout_seconds" json:"stop_timeout_seconds" yaml:"stop_timeout_seconds"`
}

// Service groups environments for a named application/service.
type Service struct {
	Name string               `mapstructure:"name" json:"name" yaml:"name"`
	Envs map[string]EnvConfig `mapstructure:"envs" json:"envs" yaml:"envs"`
}

// EnvConfig defines the per-environment SSM forwarding target.
type EnvConfig struct {
	TargetInstanceID string `mapstructure:"target_instance_id" json:"target_instance_id" yaml:"target_instance_id"`
	RemoteHost       string `mapstructure:"remote_host" json:"remote_host" yaml:"remote_host"`
	RemotePort       int    `mapstructure:"remote_port" json:"remote_port" yaml:"remote_port"`
	LocalPort        int    `mapstructure:"local_port" json:"local_port" yaml:"local_port"`
}

// Merged returns defaults with non-zero values from override applied.
func (d Defaults) Merged(override Defaults) Defaults {
	merged := d

	if override.Region != "" {
		merged.Region = override.Region
	}
	if override.Profile != "" {
		merged.Profile = override.Profile
	}
	if override.Bind != "" {
		merged.Bind = override.Bind
	}
	if len(override.PortRange) > 0 {
		merged.PortRange = append([]int(nil), override.PortRange...)
	}
	if override.StartupTimeoutSeconds != 0 {
		merged.StartupTimeoutSeconds = override.StartupTimeoutSeconds
	}
	if override.StopTimeoutSeconds != 0 {
		merged.StopTimeoutSeconds = override.StopTimeoutSeconds
	}

	return merged
}

// EffectiveDefaults ensures default values used by dbx are present.
func (c *Config) EffectiveDefaults() Defaults {
	defaults := Defaults{
		Bind:                  "127.0.0.1",
		PortRange:             []int{5500, 5999},
		StartupTimeoutSeconds: 15,
		StopTimeoutSeconds:    5,
	}
	if c == nil {
		return defaults
	}

	return defaults.Merged(c.Defaults)
}
