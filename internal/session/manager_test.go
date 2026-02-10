package session

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func withManagerTestSeams(t *testing.T, cmdFn func(context.Context, string, ...string) *exec.Cmd) {
	t.Helper()

	prevExec := execCommandContext
	prevWait := waitForPortFn
	prevPort := portAvailableFn
	execCommandContext = cmdFn
	waitForPortFn = func(bind string, port int, timeout time.Duration) error {
		return nil
	}
	portAvailableFn = func(bind string, port int) error {
		return nil
	}
	t.Cleanup(func() {
		execCommandContext = prevExec
		waitForPortFn = prevWait
		portAvailableFn = prevPort
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

func TestManagerStopWaitsForPortRelease(t *testing.T) {
	withManagerTestSeams(t, fakeLongRunningCommand)

	var calls atomic.Int32
	var stopping atomic.Bool
	portAvailableFn = func(bind string, port int) error {
		if !stopping.Load() {
			return nil
		}
		if bind == "127.0.0.1" && port == 5515 && calls.Add(1) <= 3 {
			return errors.New("address in use")
		}
		return nil
	}

	m := NewManager()
	m.defaultStopWait = 2 * time.Second
	key := NewSessionKey("service3", "dev")

	if _, err := m.Start(startOpts("service3", "dev", 5515)); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	stopping.Store(true)
	if err := m.Stop(key); err != nil {
		t.Fatalf("stop failed waiting for release: %v", err)
	}
	if got := calls.Load(); got < 4 {
		t.Fatalf("expected port checks during stop, got %d", got)
	}
}

func TestManagerStopFailsWhenPortStaysBusy(t *testing.T) {
	withManagerTestSeams(t, fakeLongRunningCommand)

	var stopping atomic.Bool
	portAvailableFn = func(bind string, port int) error {
		if !stopping.Load() {
			return nil
		}
		if bind == "127.0.0.1" && port == 5516 {
			return errors.New("address in use")
		}
		return nil
	}

	m := NewManager()
	m.defaultStopWait = 300 * time.Millisecond
	key := NewSessionKey("service4", "dev")

	if _, err := m.Start(startOpts("service4", "dev", 5516)); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	stopping.Store(true)

	err := m.Stop(key)
	if err == nil {
		t.Fatal("expected stop error when port remains busy")
	}
	want := fmt.Sprintf("%s: process stopped but local port 127.0.0.1:5516 is still in use", key)
	if err.Error() != want {
		t.Fatalf("unexpected stop error, want %q got %q", want, err.Error())
	}
}
