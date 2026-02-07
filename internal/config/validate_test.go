package config

import (
	"strings"
	"testing"
)

func validConfig() *Config {
	return &Config{
		Defaults: Defaults{
			Bind:      "127.0.0.1",
			PortRange: []int{5500, 5599},
		},
		Services: []Service{
			{
				Name: "service1",
				Envs: map[string]EnvConfig{
					"dev": {
						TargetInstanceID: "i-1",
						RemoteHost:       "db.internal",
						RemotePort:       5432,
					},
				},
			},
		},
	}
}

func TestValidateLocalPort(t *testing.T) {
	tests := []struct {
		name      string
		localPort int
		wantErr   bool
		wantPath  string
	}{
		{name: "unset is valid", localPort: 0, wantErr: false},
		{name: "in range is valid", localPort: 5432, wantErr: false},
		{name: "negative is invalid", localPort: -1, wantErr: true, wantPath: "services[service1].envs[dev].local_port"},
		{name: "too large is invalid", localPort: 70000, wantErr: true, wantPath: "services[service1].envs[dev].local_port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			env := cfg.Services[0].Envs["dev"]
			env.LocalPort = tt.localPort
			cfg.Services[0].Envs["dev"] = env

			err := Validate(cfg)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantPath != "" && !strings.Contains(err.Error(), tt.wantPath) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantPath, err.Error())
			}
		})
	}
}
