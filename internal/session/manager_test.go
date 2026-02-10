package session

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func withManagerTestSeams(t *testing.T, cmdFn func(context.Context, string, ...string) *exec.Cmd) {
	t.Helper()

	prevExec := execCommandContext
	prevWait := waitForPortFn
	execCommandContext = cmdFn
	waitForPortFn = func(bind string, port int, timeout time.Duration) error {
		return nil
	}
	t.Cleanup(func() {
		execCommandContext = prevExec
		waitForPortFn = prevWait
	})
}

func fakeLongRunningCommand(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", "sleep 10")
}

func startOpts(service, env string, port int) StartOptions {
	return StartOptions{
		Service:          service,
		Env:              env,
		Bind:             "127.0.0.1",
		LocalPort:        port,
		TargetInstanceID: "i-123",
		RemoteHost:       "db.internal",
		RemotePort:       5432,
		StartupTimeout:   time.Second,
	}
}

func TestManagerStartStopStartSameKey(t *testing.T) {
	withManagerTestSeams(t, fakeLongRunningCommand)

	m := NewManager()
	m.defaultStopWait = 2 * time.Second
	key := NewSessionKey("service1", "dev")

	if _, err := m.Start(startOpts("service1", "dev", 5511)); err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	if err := m.Stop(key); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	if _, err := m.Start(startOpts("service1", "dev", 5511)); err != nil {
		t.Fatalf("second start failed: %v", err)
	}
}

func TestManagerStopRemovesSessionFromState(t *testing.T) {
	withManagerTestSeams(t, fakeLongRunningCommand)

	m := NewManager()
	m.defaultStopWait = 2 * time.Second
	key := NewSessionKey("service2", "qa")

	if _, err := m.Start(startOpts("service2", "qa", 5512)); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := m.Stop(key); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	if _, ok := m.Get(key); ok {
		t.Fatalf("expected session %s to be removed after stop", key)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("expected no sessions after stop, got %d", got)
	}
}

func TestManagerStopAllRemovesAllSessions(t *testing.T) {
	withManagerTestSeams(t, fakeLongRunningCommand)

	m := NewManager()
	m.defaultStopWait = 2 * time.Second

	if _, err := m.Start(startOpts("service1", "dev", 5513)); err != nil {
		t.Fatalf("start service1/dev failed: %v", err)
	}
	if _, err := m.Start(startOpts("service2", "qa", 5514)); err != nil {
		t.Fatalf("start service2/qa failed: %v", err)
	}

	if err := m.StopAll(); err != nil {
		t.Fatalf("stop all failed: %v", err)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("expected no sessions after stop-all, got %d", got)
	}

	if _, err := m.Start(startOpts("service1", "dev", 5513)); err != nil {
		t.Fatalf("re-start after stop-all failed: %v", err)
	}
}
