package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fredyranthun/db/internal/session"
)

type fakeAppManager struct {
	stopAllCalls int
}

func (f *fakeAppManager) Start(opts session.StartOptions) (*session.Session, error) {
	return session.NewSession(opts.Service, opts.Env), nil
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
