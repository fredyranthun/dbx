package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fredyranthun/db/internal/config"
	"github.com/fredyranthun/db/internal/session"
)

type fakeManager struct {
	listSessions []session.SessionSummary
	logs         map[session.SessionKey][]string

	startCalls []session.StartOptions
	stopCalls  []session.SessionKey

	nextSubID uint64
	subs      map[session.SessionKey]map[uint64]chan string
	unsubbed  map[session.SessionKey][]uint64
}

type strictManager struct {
	*fakeManager
}

func newStrictManager() *strictManager {
	return &strictManager{fakeManager: newFakeManager()}
}

func newFakeManager() *fakeManager {
	return &fakeManager{
		logs:     map[session.SessionKey][]string{},
		subs:     map[session.SessionKey]map[uint64]chan string{},
		unsubbed: map[session.SessionKey][]uint64{},
	}
}

func (f *fakeManager) List() []session.SessionSummary {
	out := make([]session.SessionSummary, len(f.listSessions))
	copy(out, f.listSessions)
	return out
}

func (f *fakeManager) Start(opts session.StartOptions) (*session.Session, error) {
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

func (f *fakeManager) Stop(key session.SessionKey) error {
	f.stopCalls = append(f.stopCalls, key)
	return nil
}

func (f *fakeManager) StopAll() error {
	return nil
}

func (f *fakeManager) LastLogs(key session.SessionKey, n int) ([]string, error) {
	lines := f.logs[key]
	if n <= 0 || len(lines) == 0 {
		return nil, nil
	}
	if n > len(lines) {
		n = len(lines)
	}
	out := make([]string, n)
	copy(out, lines[len(lines)-n:])
	return out, nil
}

func (f *fakeManager) SubscribeLogs(key session.SessionKey, buffer int) (uint64, <-chan string, error) {
	if buffer < 0 {
		buffer = 0
	}
	f.nextSubID++
	id := f.nextSubID
	if _, ok := f.subs[key]; !ok {
		f.subs[key] = map[uint64]chan string{}
	}
	ch := make(chan string, buffer)
	f.subs[key][id] = ch
	return id, ch, nil
}

func (f *fakeManager) UnsubscribeLogs(key session.SessionKey, id uint64) {
	byKey, ok := f.subs[key]
	if !ok {
		return
	}
	ch, ok := byKey[id]
	if !ok {
		return
	}
	delete(byKey, id)
	close(ch)
	f.unsubbed[key] = append(f.unsubbed[key], id)
}

func (f *fakeManager) activeSubscriptions() int {
	total := 0
	for _, byKey := range f.subs {
		total += len(byKey)
	}
	return total
}

func (f *fakeManager) firstActive(key session.SessionKey) (uint64, chan string, bool) {
	byKey, ok := f.subs[key]
	if !ok {
		return 0, nil, false
	}
	for id, ch := range byKey {
		return id, ch, true
	}
	return 0, nil, false
}

func (s *strictManager) LastLogs(key session.SessionKey, n int) ([]string, error) {
	if !s.hasSession(key) {
		return nil, fmt.Errorf("%s: session not found", key)
	}
	return s.fakeManager.LastLogs(key, n)
}

func (s *strictManager) SubscribeLogs(key session.SessionKey, buffer int) (uint64, <-chan string, error) {
	if !s.hasSession(key) {
		return 0, nil, fmt.Errorf("%s: session not found", key)
	}
	return s.fakeManager.SubscribeLogs(key, buffer)
}

func (s *strictManager) hasSession(key session.SessionKey) bool {
	for _, sess := range s.listSessions {
		if sess.Key == key {
			return true
		}
	}
	return false
}

func testConfig() *config.Config {
	return &config.Config{
		Defaults: config.Defaults{
			Bind:                  "127.0.0.1",
			PortRange:             []int{5500, 5599},
			StartupTimeoutSeconds: 5,
		},
		Services: []config.Service{
			{
				Name: "service1",
				Envs: map[string]config.EnvConfig{
					"dev": {TargetInstanceID: "i-1", RemoteHost: "db1", RemotePort: 5432, LocalPort: 55432},
				},
			},
			{
				Name: "service2",
				Envs: map[string]config.EnvConfig{
					"qa": {TargetInstanceID: "i-2", RemoteHost: "db2", RemotePort: 3306},
				},
			},
		},
	}
}

func updateModel(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	model, ok := updated.(Model)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	return model, cmd
}

func keyMsg(v string) tea.KeyMsg {
	if v == "tab" {
		return tea.KeyMsg(tea.Key{Type: tea.KeyTab})
	}
	return tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune(v)})
}

func TestModelKeyHandlingFocusAndSelection(t *testing.T) {
	m := NewModel(newFakeManager(), testConfig())

	if m.focused != PaneTargets {
		t.Fatalf("expected initial focus targets, got %s", m.focused)
	}

	m, _ = updateModel(t, m, keyMsg("tab"))
	if m.focused != PaneSessions {
		t.Fatalf("expected sessions focus, got %s", m.focused)
	}
	m, _ = updateModel(t, m, keyMsg("tab"))
	if m.focused != PaneLogs {
		t.Fatalf("expected logs focus, got %s", m.focused)
	}
	m, _ = updateModel(t, m, keyMsg("tab"))
	if m.focused != PaneTargets {
		t.Fatalf("expected focus to wrap to targets, got %s", m.focused)
	}

	m, _ = updateModel(t, m, keyMsg("j"))
	if m.targetSelected != 1 {
		t.Fatalf("expected target selection=1, got %d", m.targetSelected)
	}
	m, _ = updateModel(t, m, keyMsg("j"))
	if m.targetSelected != 1 {
		t.Fatalf("expected target selection clamped at 1, got %d", m.targetSelected)
	}
	m, _ = updateModel(t, m, keyMsg("k"))
	if m.targetSelected != 0 {
		t.Fatalf("expected target selection=0, got %d", m.targetSelected)
	}
}

func TestModelConnectAndStopDispatch(t *testing.T) {
	fm := newFakeManager()
	key := session.NewSessionKey("service1", "dev")
	fm.listSessions = []session.SessionSummary{{Key: key, Bind: "127.0.0.1", LocalPort: 5501, State: session.SessionStateRunning}}

	m := NewModel(fm, testConfig())
	m, _ = updateModel(t, m, refreshTickMsg{sessions: fm.List()})

	m, cmd := updateModel(t, m, keyMsg("c"))
	if cmd == nil {
		t.Fatal("expected connect cmd")
	}
	msg := cmd()
	m, _ = updateModel(t, m, msg)
	if len(fm.startCalls) != 1 {
		t.Fatalf("expected one start call, got %d", len(fm.startCalls))
	}
	if fm.startCalls[0].Service != "service1" || fm.startCalls[0].Env != "dev" {
		t.Fatalf("unexpected start target: %s/%s", fm.startCalls[0].Service, fm.startCalls[0].Env)
	}
	if fm.startCalls[0].LocalPort != 55432 {
		t.Fatalf("expected configured local port 55432, got %d", fm.startCalls[0].LocalPort)
	}
	if !strings.Contains(m.status, "connected") {
		t.Fatalf("expected connected status, got %q", m.status)
	}

	m, _ = updateModel(t, m, keyMsg("tab"))
	m, cmd = updateModel(t, m, keyMsg("s"))
	if cmd == nil {
		t.Fatal("expected stop cmd")
	}
	m, _ = updateModel(t, m, cmd())
	if len(fm.stopCalls) != 1 || fm.stopCalls[0] != key {
		t.Fatalf("unexpected stop calls: %v", fm.stopCalls)
	}
	if !strings.Contains(m.status, "stopped") {
		t.Fatalf("expected stopped status, got %q", m.status)
	}
}

func TestModelConnectWithoutConfiguredLocalPortUsesRangePath(t *testing.T) {
	fm := newFakeManager()
	m := NewModel(fm, testConfig())
	m.targetSelected = 1 // service2/qa has no local_port

	_, cmd := updateModel(t, m, keyMsg("c"))
	if cmd == nil {
		t.Fatal("expected connect cmd")
	}
	msg := cmd()
	_, _ = updateModel(t, m, msg)

	if len(fm.startCalls) != 1 {
		t.Fatalf("expected one start call, got %d", len(fm.startCalls))
	}
	if fm.startCalls[0].Service != "service2" || fm.startCalls[0].Env != "qa" {
		t.Fatalf("unexpected start target: %s/%s", fm.startCalls[0].Service, fm.startCalls[0].Env)
	}
	if fm.startCalls[0].LocalPort != 0 {
		t.Fatalf("expected local port unset (0), got %d", fm.startCalls[0].LocalPort)
	}
}

func TestModelFollowToggleAndSubscriptionLifecycle(t *testing.T) {
	fm := newFakeManager()
	cfg := testConfig()
	key1 := session.NewSessionKey("service1", "dev")
	key2 := session.NewSessionKey("service2", "qa")
	fm.listSessions = []session.SessionSummary{
		{Key: key1, State: session.SessionStateRunning},
		{Key: key2, State: session.SessionStateRunning},
	}
	fm.logs[key1] = []string{"a1", "a2"}
	fm.logs[key2] = []string{"b1"}

	m := NewModel(fm, cfg)
	m, _ = updateModel(t, m, refreshTickMsg{sessions: fm.List()})
	if len(m.targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(m.targets))
	}

	m, cmd := updateModel(t, m, keyMsg("l"))
	if !m.logFollow {
		t.Fatal("expected follow enabled")
	}
	if fm.activeSubscriptions() != 1 {
		t.Fatalf("expected 1 active subscription, got %d", fm.activeSubscriptions())
	}
	if cmd == nil {
		t.Fatal("expected log reader cmd")
	}

	subID, subCh, ok := fm.firstActive(key1)
	if !ok {
		t.Fatalf("expected active subscription for %s", key1)
	}
	subCh <- "live-line"
	m, cmd = updateModel(t, m, cmd())
	if got := m.logBuffer[len(m.logBuffer)-1]; got != "live-line" {
		t.Fatalf("expected live log line appended, got %q", got)
	}
	if cmd == nil {
		t.Fatal("expected chained log reader cmd")
	}

	m, _ = updateModel(t, m, keyMsg("j"))
	if fm.activeSubscriptions() != 1 {
		t.Fatalf("expected one active subscription after target switch, got %d", fm.activeSubscriptions())
	}
	if len(fm.unsubbed[key1]) != 1 || fm.unsubbed[key1][0] != subID {
		t.Fatalf("expected old subscription to be removed for %s", key1)
	}
	if m.logKey != key2 {
		t.Fatalf("expected log key %s, got %s", key2, m.logKey)
	}

	m, _ = updateModel(t, m, keyMsg("l"))
	if m.logFollow {
		t.Fatal("expected follow disabled")
	}
	if fm.activeSubscriptions() != 0 {
		t.Fatalf("expected 0 active subscriptions after disabling follow, got %d", fm.activeSubscriptions())
	}
}

func TestModelQuitClosesLogSubscription(t *testing.T) {
	fm := newFakeManager()
	key := session.NewSessionKey("service1", "dev")
	fm.listSessions = []session.SessionSummary{
		{Key: key, State: session.SessionStateRunning},
	}
	m := NewModel(fm, testConfig())
	m, _ = updateModel(t, m, refreshTickMsg{sessions: fm.List()})

	m, _ = updateModel(t, m, keyMsg("l"))
	if fm.activeSubscriptions() != 1 {
		t.Fatalf("expected active subscription before quit, got %d", fm.activeSubscriptions())
	}

	mAny, cmd := m.handleKey(keyMsg("q"))
	m = mAny.(Model)
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if fm.activeSubscriptions() != 0 {
		t.Fatalf("expected subscriptions closed on quit, got %d", fm.activeSubscriptions())
	}
	if m.logSubID != 0 {
		t.Fatalf("expected cleared subscription id, got %d", m.logSubID)
	}
	if fmt.Sprintf("%p", cmd) != fmt.Sprintf("%p", tea.Quit) {
		t.Fatal("expected tea.Quit command")
	}
}

func TestModelSyncLogsNoSessionDoesNotSetError(t *testing.T) {
	sm := newStrictManager()
	m := NewModel(sm, testConfig())

	m, _ = updateModel(t, m, refreshTickMsg{sessions: sm.List()})

	if m.statusLevel == statusError {
		t.Fatalf("expected non-error status level, got %s (%q)", m.statusLevel, m.status)
	}
	if strings.Contains(m.status, "failed to load logs") || strings.Contains(m.status, "session not found") {
		t.Fatalf("expected no missing-session log error status, got %q", m.status)
	}
	if len(m.logBuffer) != 0 {
		t.Fatalf("expected empty log buffer, got %d lines", len(m.logBuffer))
	}
}

func TestModelFollowNoSessionDoesNotSetErrorOrSubscribe(t *testing.T) {
	sm := newStrictManager()
	m := NewModel(sm, testConfig())

	m, _ = updateModel(t, m, keyMsg("l"))

	if m.statusLevel == statusError {
		t.Fatalf("expected non-error status level, got %s (%q)", m.statusLevel, m.status)
	}
	if strings.Contains(m.status, "failed to follow logs") || strings.Contains(m.status, "session not found") {
		t.Fatalf("expected no missing-session follow error status, got %q", m.status)
	}
	if sm.activeSubscriptions() != 0 {
		t.Fatalf("expected no subscriptions without active session, got %d", sm.activeSubscriptions())
	}
}
