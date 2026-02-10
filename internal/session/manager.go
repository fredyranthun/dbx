package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultPortRangeMin   = 5500
	defaultPortRangeMax   = 5999
	defaultStartupTimeout = 15 * time.Second
	defaultStopTimeout    = 5 * time.Second
	logTailLinesOnError   = 20
)

var (
	errSessionNotFound = errors.New("session not found")
	execCommandContext = exec.CommandContext
	waitForPortFn      = WaitForPort
	portAvailableFn    = ValidatePortAvailable
)

// StartOptions contains the parameters required to start one session.
type StartOptions struct {
	Service          string
	Env              string
	Bind             string
	LocalPort        int
	PortMin          int
	PortMax          int
	TargetInstanceID string
	RemoteHost       string
	RemotePort       int
	Region           string
	Profile          string
	StartupTimeout   time.Duration
}

// SessionSummary is a read-only snapshot used by list output.
type SessionSummary struct {
	Key       SessionKey
	Service   string
	Env       string
	Bind      string
	LocalPort int
	PID       int
	State     SessionState
	StartTime time.Time
	Uptime    time.Duration
	LastError string
}

// Manager tracks active forwarding sessions and their lifecycle.
type Manager struct {
	mu sync.RWMutex

	sessions map[SessionKey]*Session

	defaultPortMin   int
	defaultPortMax   int
	defaultStartWait time.Duration
	defaultStopWait  time.Duration
}

func NewManager() *Manager {
	return &Manager{
		sessions:         make(map[SessionKey]*Session),
		defaultPortMin:   defaultPortRangeMin,
		defaultPortMax:   defaultPortRangeMax,
		defaultStartWait: defaultStartupTimeout,
		defaultStopWait:  defaultStopTimeout,
	}
}

// Start creates and starts an aws ssm start-session process.
func (m *Manager) Start(opts StartOptions) (*Session, error) {
	if m == nil {
		return nil, errors.New("manager is nil")
	}
	if opts.Service == "" || opts.Env == "" {
		return nil, errors.New("service and env are required")
	}
	if opts.TargetInstanceID == "" || opts.RemoteHost == "" || opts.RemotePort == 0 {
		return nil, errors.New("target_instance_id, remote_host and remote_port are required")
	}
	if opts.Bind == "" {
		opts.Bind = "127.0.0.1"
	}
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = m.defaultStartWait
	}

	key := NewSessionKey(opts.Service, opts.Env)

	m.mu.Lock()
	if existing, exists := m.sessions[key]; exists {
		if existing == nil || existing.State == SessionStateStopped {
			delete(m.sessions, key)
		} else {
			m.mu.Unlock()
			return nil, fmt.Errorf("%s: session already exists", key)
		}
	}

	port, err := m.selectPortLocked(opts)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("%s: failed to allocate local port: %w", key, err)
	}

	s := NewSession(opts.Service, opts.Env)
	s.Bind = opts.Bind
	s.LocalPort = port
	s.RemoteHost = opts.RemoteHost
	s.RemotePort = opts.RemotePort
	s.TargetInstanceID = opts.TargetInstanceID
	s.Region = opts.Region
	s.Profile = opts.Profile
	s.StartTime = time.Now()
	s.State = SessionStateStarting
	m.sessions[key] = s
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	args := BuildSSMPortForwardArgs(
		opts.TargetInstanceID,
		opts.RemoteHost,
		opts.RemotePort,
		port,
		opts.Region,
		opts.Profile,
	)
	cmd := execCommandContext(ctx, "aws", args...)
	configureCommandForPlatform(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		m.failStart(key, fmt.Errorf("failed to capture stdout: %w", err))
		startErr := m.startErrorWithLogs(key, err)
		m.removeSession(key)
		return nil, startErr
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		m.failStart(key, fmt.Errorf("failed to capture stderr: %w", err))
		startErr := m.startErrorWithLogs(key, err)
		m.removeSession(key)
		return nil, startErr
	}

	if err := cmd.Start(); err != nil {
		cancel()
		m.failStart(key, fmt.Errorf("failed to start aws command: %w", err))
		startErr := m.startErrorWithLogs(key, err)
		m.removeSession(key)
		return nil, startErr
	}

	m.mu.Lock()
	s.cmd = cmd
	s.cancel = cancel
	if cmd.Process != nil {
		s.PID = cmd.Process.Pid
	}
	m.mu.Unlock()

	go m.pipeLogs(key, stdout)
	go m.pipeLogs(key, stderr)
	go m.waitProcess(key, cmd)

	if err := m.waitUntilReady(key, opts.Bind, port, opts.StartupTimeout); err != nil {
		startErr := m.startErrorWithLogs(key, err)
		stopErr := m.Stop(key)
		if stopErr != nil {
			return nil, fmt.Errorf("%v\ncleanup error: %w", startErr, stopErr)
		}
		return nil, startErr
	}

	m.mu.Lock()
	if current, ok := m.sessions[key]; ok {
		current.State = SessionStateRunning
	}
	out := m.copySessionLocked(key)
	m.mu.Unlock()

	return out, nil
}

// Stop requests graceful shutdown and forces kill after timeout.
func (m *Manager) Stop(key SessionKey) error {
	if m == nil {
		return errors.New("manager is nil")
	}

	m.mu.Lock()
	s, ok := m.sessions[key]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%s: %w", key, errSessionNotFound)
	}
	if s.State == SessionStateStopped {
		delete(m.sessions, key)
		m.mu.Unlock()
		return nil
	}
	if s.State == SessionStateError {
		s.CloseLogSubscribers()
		delete(m.sessions, key)
		m.mu.Unlock()
		return nil
	}
	if s.State != SessionStateStopping {
		s.State = SessionStateStopping
	}
	cmd := s.cmd
	m.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		m.mu.Lock()
		m.removeSessionLocked(key)
		m.mu.Unlock()
		return nil
	}

	if err := interruptSessionProcess(cmd); err != nil {
		return fmt.Errorf("%s: failed to interrupt process: %w", key, err)
	}

	if m.waitForState(key, SessionStateStopped, m.defaultStopWait) {
		if err := m.waitUntilPortReleased(s.Bind, s.LocalPort, m.defaultStopWait); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		return nil
	}

	if err := killSessionProcess(cmd); err != nil {
		return fmt.Errorf("%s: failed to kill process: %w", key, err)
	}

	if !m.waitForState(key, SessionStateStopped, 2*time.Second) {
		return fmt.Errorf("%s: session did not stop within timeout", key)
	}
	if err := m.waitUntilPortReleased(s.Bind, s.LocalPort, 2*time.Second); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}

	return nil
}

// StopAll stops all known sessions and returns a joined error if any stop fails.
func (m *Manager) StopAll() error {
	if m == nil {
		return errors.New("manager is nil")
	}

	m.mu.RLock()
	keys := make([]SessionKey, 0, len(m.sessions))
	for key := range m.sessions {
		keys = append(keys, key)
	}
	m.mu.RUnlock()

	var errs []error
	for _, key := range keys {
		if err := m.Stop(key); err != nil {
			if errors.Is(err, errSessionNotFound) {
				continue
			}
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// List returns snapshots ordered by key.
func (m *Manager) List() []SessionSummary {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	keys := make([]SessionKey, 0, len(m.sessions))
	for key := range m.sessions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	out := make([]SessionSummary, 0, len(keys))
	now := time.Now()
	for _, key := range keys {
		s := m.sessions[key]
		if s == nil {
			continue
		}
		uptime := time.Duration(0)
		if !s.StartTime.IsZero() {
			uptime = now.Sub(s.StartTime)
		}
		out = append(out, SessionSummary{
			Key:       s.Key,
			Service:   s.Service,
			Env:       s.Env,
			Bind:      s.Bind,
			LocalPort: s.LocalPort,
			PID:       s.PID,
			State:     s.State,
			StartTime: s.StartTime,
			Uptime:    uptime,
			LastError: s.LastError,
		})
	}
	m.mu.RUnlock()

	return out
}

// Get returns a copy of the current session snapshot.
func (m *Manager) Get(key SessionKey) (*Session, bool) {
	if m == nil {
		return nil, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[key]
	if !ok || s == nil {
		return nil, false
	}

	cp := *s
	return &cp, true
}

func (m *Manager) selectPortLocked(opts StartOptions) (int, error) {
	if opts.LocalPort > 0 {
		if m.portReservedLocked(opts.Bind, opts.LocalPort) {
			return 0, fmt.Errorf("requested port %d already used by another session", opts.LocalPort)
		}
		if err := portAvailableFn(opts.Bind, opts.LocalPort); err != nil {
			return 0, err
		}
		return opts.LocalPort, nil
	}

	min := opts.PortMin
	max := opts.PortMax
	if min == 0 {
		min = m.defaultPortMin
	}
	if max == 0 {
		max = m.defaultPortMax
	}
	if min > max {
		return 0, fmt.Errorf("invalid port range %d-%d", min, max)
	}

	for port := min; port <= max; port++ {
		if m.portReservedLocked(opts.Bind, port) {
			continue
		}
		if err := portAvailableFn(opts.Bind, port); err == nil {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no free port available on %s in range %d-%d", opts.Bind, min, max)
}

func (m *Manager) portReservedLocked(bind string, port int) bool {
	for _, s := range m.sessions {
		if s == nil {
			continue
		}
		if s.Bind == bind && s.LocalPort == port && s.State != SessionStateStopped {
			return true
		}
	}
	return false
}

func (m *Manager) waitUntilReady(key SessionKey, bind string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("%s: timed out waiting for local port readiness", key)
		}

		interval := 500 * time.Millisecond
		if remaining < interval {
			interval = remaining
		}
		if err := waitForPortFn(bind, port, interval); err == nil {
			return nil
		}

		m.mu.RLock()
		s, ok := m.sessions[key]
		state := SessionStateStopped
		lastErr := ""
		if ok && s != nil {
			state = s.State
			lastErr = s.LastError
		}
		m.mu.RUnlock()

		if !ok {
			return fmt.Errorf("%s: session no longer exists", key)
		}
		if state == SessionStateError {
			if lastErr == "" {
				return fmt.Errorf("%s: aws process exited before readiness", key)
			}
			return fmt.Errorf("%s: %s", key, lastErr)
		}
		if state == SessionStateStopped {
			return fmt.Errorf("%s: session stopped before readiness", key)
		}
	}
}

func (m *Manager) waitForState(key SessionKey, desired SessionState, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.mu.RLock()
		s, ok := m.sessions[key]
		state := SessionStateStopped
		if ok && s != nil {
			state = s.State
		}
		m.mu.RUnlock()
		if state == desired {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (m *Manager) waitProcess(key SessionKey, cmd *exec.Cmd) {
	err := cmd.Wait()

	m.mu.Lock()
	s, ok := m.sessions[key]
	if !ok || s == nil {
		m.mu.Unlock()
		return
	}
	if s.State == SessionStateStopping {
		m.removeSessionLocked(key)
		m.mu.Unlock()
		return
	} else if err != nil {
		s.AppendLog(fmt.Sprintf("process exited: %v", err))
	} else {
		s.AppendLog("process exited cleanly")
	}
	m.removeSessionLocked(key)
	m.mu.Unlock()
}

func (m *Manager) waitUntilPortReleased(bind string, port int, timeout time.Duration) error {
	if port <= 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := portAvailableFn(bind, port); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("process stopped but local port %s:%d is still in use", bind, port)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *Manager) pipeLogs(key SessionKey, src io.ReadCloser) {
	defer src.Close()

	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		line := scanner.Text()
		m.mu.RLock()
		s, ok := m.sessions[key]
		m.mu.RUnlock()
		if !ok || s == nil {
			return
		}
		s.AppendLog(line)
	}

	if err := scanner.Err(); err != nil {
		m.mu.RLock()
		s, ok := m.sessions[key]
		m.mu.RUnlock()
		if ok && s != nil {
			s.AppendLog(fmt.Sprintf("log stream error: %v", err))
		}
	}
}

func (m *Manager) failStart(key SessionKey, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[key]; ok && s != nil {
		s.State = SessionStateError
		s.LastError = err.Error()
	}
}

func (m *Manager) startErrorWithLogs(key SessionKey, startErr error) error {
	m.mu.RLock()
	s, ok := m.sessions[key]
	var logs []string
	if ok && s != nil {
		logs = s.LastLogs(logTailLinesOnError)
	}
	m.mu.RUnlock()

	if len(logs) == 0 {
		return fmt.Errorf("%s: failed to start session: %w", key, startErr)
	}

	return fmt.Errorf("%s: failed to start session: %w\nrecent logs:\n%s", key, startErr, strings.Join(logs, "\n"))
}

func (m *Manager) copySessionLocked(key SessionKey) *Session {
	s, ok := m.sessions[key]
	if !ok || s == nil {
		return nil
	}
	cp := *s
	return &cp
}

func (m *Manager) removeSession(key SessionKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeSessionLocked(key)
}

func (m *Manager) removeSessionLocked(key SessionKey) {
	s, ok := m.sessions[key]
	if !ok || s == nil {
		delete(m.sessions, key)
		return
	}
	s.State = SessionStateStopped
	s.CloseLogSubscribers()
	delete(m.sessions, key)
}
