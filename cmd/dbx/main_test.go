package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fredyranthun/db/internal/session"
)

type fakeAppManager struct {
	stopAllCalls int
	startCalls   []session.StartOptions
}

func (f *fakeAppManager) Start(opts session.StartOptions) (*session.Session, error) {
	f.startCalls = append(f.startCalls, opts)
	s := session.NewSession(opts.Service, opts.Env)
	s.Bind = opts.Bind
	if opts.LocalPort == 0 {
		s.LocalPort = 5500
	} else {
		s.LocalPort = opts.LocalPort
	}
	return s, nil
}

func (f *fakeAppManager) Stop(key session.SessionKey) error {
	return nil
}

func (f *fakeAppManager) StopAll() error {
	f.stopAllCalls++
	return nil
}

func (f *fakeAppManager) List() []session.SessionSummary {
	return nil
}

func (f *fakeAppManager) Get(key session.SessionKey) (*session.Session, bool) {
	return nil, false
}

func (f *fakeAppManager) LastLogs(key session.SessionKey, n int) ([]string, error) {
	return nil, nil
}

func (f *fakeAppManager) SubscribeLogs(key session.SessionKey, buffer int) (uint64, <-chan string, error) {
	ch := make(chan string)
	close(ch)
	return 1, ch, nil
}

func (f *fakeAppManager) UnsubscribeLogs(key session.SessionKey, id uint64) {}

type fakeTeaRunner struct{}

func (f fakeTeaRunner) Run() (tea.Model, error) {
	return nil, nil
}

func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `defaults:
  region: sa-east-1
  profile: corp
  bind: "127.0.0.1"
  port_range: [5500, 5999]
  startup_timeout_seconds: 1
  stop_timeout_seconds: 1
services:
  - name: service1
    envs:
      dev:
        target_instance_id: "i-0123456789abcdef0"
        remote_host: "db.internal"
        remote_port: 5432
        local_port: 55432
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeTestConfigWithoutLocalPort(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `defaults:
  region: sa-east-1
  profile: corp
  bind: "127.0.0.1"
  port_range: [5500, 5999]
  startup_timeout_seconds: 1
  stop_timeout_seconds: 1
services:
  - name: service1
    envs:
      dev:
        target_instance_id: "i-0123456789abcdef0"
        remote_host: "db.internal"
        remote_port: 5432
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestUICmdQuitTriggersCleanupByDefault(t *testing.T) {
	manager := &fakeAppManager{}
	a := &app{manager: manager, configPath: writeTestConfig(t)}

	prevRunner := newTeaRunner
	newTeaRunner = func(model tea.Model) teaRunner {
		return fakeTeaRunner{}
	}
	defer func() { newTeaRunner = prevRunner }()

	cmd := a.newUICmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("ui command failed: %v", err)
	}

	if manager.stopAllCalls != 1 {
		t.Fatalf("expected StopAll to be called once, got %d", manager.stopAllCalls)
	}
}

func TestUICmdQuitSkipsCleanupWhenNoCleanupEnabled(t *testing.T) {
	manager := &fakeAppManager{}
	a := &app{manager: manager, configPath: writeTestConfig(t), noCleanup: true}

	prevRunner := newTeaRunner
	newTeaRunner = func(model tea.Model) teaRunner {
		return fakeTeaRunner{}
	}
	defer func() { newTeaRunner = prevRunner }()

	cmd := a.newUICmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("ui command failed: %v", err)
	}

	if manager.stopAllCalls != 0 {
		t.Fatalf("expected StopAll to be skipped, got %d calls", manager.stopAllCalls)
	}
}

func TestVersionCommandPrintsMetadata(t *testing.T) {
	manager := &fakeAppManager{}
	a := &app{manager: manager}

	prevVersion := version
	prevCommit := commit
	prevDate := date
	version = "v1.2.3"
	commit = "abc123"
	date = "2026-02-07T00:00:00Z"
	defer func() {
		version = prevVersion
		commit = prevCommit
		date = prevDate
	}()

	root := newRootCmd(a)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	got := out.String()
	want := "v1.2.3 (commit=abc123 date=2026-02-07T00:00:00Z)"
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got %q", want, got)
	}
}

func TestConnectUsesEnvLocalPortWhenNoFlag(t *testing.T) {
	manager := &fakeAppManager{}
	a := &app{manager: manager}
	root := newRootCmd(a)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--config", writeTestConfig(t), "connect", "service1", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("connect command failed: %v", err)
	}
	if len(manager.startCalls) != 1 {
		t.Fatalf("expected one start call, got %d", len(manager.startCalls))
	}
	if got := manager.startCalls[0].LocalPort; got != 55432 {
		t.Fatalf("expected local port 55432 from config, got %d", got)
	}
}

func TestConnectPortFlagOverridesEnvLocalPort(t *testing.T) {
	manager := &fakeAppManager{}
	a := &app{manager: manager}
	root := newRootCmd(a)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--config", writeTestConfig(t), "connect", "service1", "dev", "--port", "55499"})

	if err := root.Execute(); err != nil {
		t.Fatalf("connect command failed: %v", err)
	}
	if len(manager.startCalls) != 1 {
		t.Fatalf("expected one start call, got %d", len(manager.startCalls))
	}
	if got := manager.startCalls[0].LocalPort; got != 55499 {
		t.Fatalf("expected local port 55499 from --port, got %d", got)
	}
}

func TestConnectLeavesLocalPortUnsetWhenConfigAndFlagAreAbsent(t *testing.T) {
	manager := &fakeAppManager{}
	a := &app{manager: manager}
	root := newRootCmd(a)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--config", writeTestConfigWithoutLocalPort(t), "connect", "service1", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("connect command failed: %v", err)
	}
	if len(manager.startCalls) != 1 {
		t.Fatalf("expected one start call, got %d", len(manager.startCalls))
	}
	if got := manager.startCalls[0].LocalPort; got != 0 {
		t.Fatalf("expected local port unset (0), got %d", got)
	}
}
