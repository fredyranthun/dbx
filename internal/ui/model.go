package ui

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fredyranthun/db/internal/config"
	"github.com/fredyranthun/db/internal/session"
)

const defaultRefreshInterval = 1 * time.Second

type Pane string

type statusLevel string

const (
	PaneTargets  Pane = "targets"
	PaneSessions Pane = "sessions"
	PaneLogs     Pane = "logs"
)

const (
	statusInfo    statusLevel = "info"
	statusSuccess statusLevel = "success"
	statusWarn    statusLevel = "warn"
	statusError   statusLevel = "error"
)

type Target struct {
	Service string
	Env     string
	Key     session.SessionKey
}

type refreshTickMsg struct {
	sessions []session.SessionSummary
}

type connectResultMsg struct {
	key      session.SessionKey
	endpoint string
	err      error
}

type stopResultMsg struct {
	key session.SessionKey
	err error
}

type stopAllResultMsg struct {
	err error
}

type logLineMsg struct {
	key    session.SessionKey
	subID  uint64
	line   string
	closed bool
}

type sessionManager interface {
	List() []session.SessionSummary
	Start(opts session.StartOptions) (*session.Session, error)
	Stop(key session.SessionKey) error
	StopAll() error
	LastLogs(key session.SessionKey, n int) ([]string, error)
	SubscribeLogs(key session.SessionKey, buffer int) (uint64, <-chan string, error)
	UnsubscribeLogs(key session.SessionKey, id uint64)
}

type Model struct {
	width  int
	height int

	targets         []Target
	sessions        []session.SessionSummary
	targetSelected  int
	sessionSelected int
	focused         Pane
	status          string
	statusLevel     statusLevel
	manager         sessionManager
	cfg             *config.Config
	defaults        config.Defaults
	refreshIn       time.Duration
	logFollow       bool
	logLines        int
	logKey          session.SessionKey
	logBuffer       []string
	logSubKey       session.SessionKey
	logSubID        uint64
	logSubCh        <-chan string
	logReadActive   bool
}

func NewModel(manager sessionManager, cfg *config.Config) Model {
	targets := configuredTargets(cfg)
	defaults := config.Defaults{}
	if cfg != nil {
		defaults = cfg.EffectiveDefaults()
	}

	status := "ready"
	level := statusInfo
	if len(targets) == 0 {
		status = "no configured targets found"
		level = statusWarn
	}

	return Model{
		targets:     targets,
		focused:     PaneTargets,
		status:      status,
		statusLevel: level,
		manager:     manager,
		cfg:         cfg,
		defaults:    defaults,
		refreshIn:   defaultRefreshInterval,
		logLines:    50,
	}
}

func (m Model) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		return m.handleKey(msg)
	case refreshTickMsg:
		m.sessions = msg.sessions
		m.clampSelections()
		m.syncLogs(false)
		return m, m.refreshCmd()
	case connectResultMsg:
		if msg.err != nil {
			m.statusLevel = statusError
			m.status = fmt.Sprintf("%s: connect failed: %v", msg.key, msg.err)
		} else {
			m.statusLevel = statusSuccess
			m.status = fmt.Sprintf("%s: connected (%s)", msg.key, msg.endpoint)
		}
		return m, m.refreshNowCmd()
	case stopResultMsg:
		if msg.err != nil {
			m.statusLevel = statusError
			m.status = fmt.Sprintf("%s: stop failed: %v", msg.key, msg.err)
		} else {
			m.statusLevel = statusSuccess
			m.status = fmt.Sprintf("%s: stopped", msg.key)
		}
		return m, m.refreshNowCmd()
	case stopAllResultMsg:
		if msg.err != nil {
			m.statusLevel = statusError
			m.status = fmt.Sprintf("stop all failed: %v", msg.err)
		} else {
			m.statusLevel = statusSuccess
			m.status = "stopped all sessions"
		}
		return m, m.refreshNowCmd()
	case logLineMsg:
		if msg.subID == 0 || msg.subID != m.logSubID || msg.key != m.logSubKey {
			return m, nil
		}
		if msg.closed {
			m.logSubID = 0
			m.logSubCh = nil
			m.logReadActive = false
			return m, nil
		}
		m.logBuffer = append(m.logBuffer, msg.line)
		if len(m.logBuffer) > session.DefaultRingBufferLines {
			m.logBuffer = m.logBuffer[len(m.logBuffer)-session.DefaultRingBufferLines:]
		}
		return m, m.logReadCmd(msg.key, msg.subID, m.logSubCh)
	}

	return m, nil
}

func (m Model) View() string {
	return RenderView(m)
}

func (m Model) refreshCmd() tea.Cmd {
	return m.refreshWithDelay(m.refreshIn)
}

func (m Model) refreshNowCmd() tea.Cmd {
	return m.refreshWithDelay(0)
}

func (m Model) refreshWithDelay(delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		if delay > 0 {
			time.Sleep(delay)
		}
		if m.manager == nil {
			return refreshTickMsg{}
		}
		return refreshTickMsg{sessions: m.manager.List()}
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.closeLogSubscription()
		return m, tea.Quit
	case "tab":
		m.cycleFocus()
		m.statusLevel = statusInfo
		m.status = fmt.Sprintf("focus: %s", m.focused)
		m.syncLogs(true)
		return m, m.ensureLogReaderCmd()
	case "j", "down":
		m.moveSelection(1)
		m.syncLogs(true)
		return m, m.ensureLogReaderCmd()
	case "k", "up":
		m.moveSelection(-1)
		m.syncLogs(true)
		return m, m.ensureLogReaderCmd()
	case "c":
		cmd := m.connectSelectedCmd()
		if cmd == nil {
			if len(m.targets) == 0 {
				m.statusLevel = statusWarn
				m.status = "no target selected"
			}
			return m, nil
		}
		m.statusLevel = statusInfo
		m.status = fmt.Sprintf("%s: connecting...", m.currentTargetKey())
		return m, cmd
	case "s":
		cmd := m.stopSelectedCmd()
		if cmd == nil {
			if len(m.sessions) == 0 {
				m.statusLevel = statusWarn
				m.status = "no running session selected"
			}
			return m, nil
		}
		m.statusLevel = statusInfo
		m.status = fmt.Sprintf("%s: stopping...", m.currentSessionKey())
		return m, cmd
	case "S":
		if m.manager == nil {
			m.statusLevel = statusError
			m.status = "session manager unavailable"
			return m, nil
		}
		m.statusLevel = statusInfo
		m.status = "stopping all sessions..."
		return m, m.stopAllCmd()
	case "l":
		m.logFollow = !m.logFollow
		m.statusLevel = statusInfo
		if m.logFollow {
			m.status = "log follow enabled"
		} else {
			m.status = "log follow disabled"
		}
		m.syncLogs(true)
		return m, m.ensureLogReaderCmd()
	}

	return m, nil
}

func (m *Model) cycleFocus() {
	switch m.focused {
	case PaneTargets:
		m.focused = PaneSessions
	case PaneSessions:
		m.focused = PaneLogs
	default:
		m.focused = PaneTargets
	}
}

func (m *Model) moveSelection(delta int) {
	switch m.focused {
	case PaneTargets:
		if len(m.targets) == 0 {
			m.targetSelected = 0
			return
		}
		m.targetSelected += delta
		if m.targetSelected < 0 {
			m.targetSelected = 0
		}
		if m.targetSelected >= len(m.targets) {
			m.targetSelected = len(m.targets) - 1
		}
	case PaneSessions:
		if len(m.sessions) == 0 {
			m.sessionSelected = 0
			return
		}
		m.sessionSelected += delta
		if m.sessionSelected < 0 {
			m.sessionSelected = 0
		}
		if m.sessionSelected >= len(m.sessions) {
			m.sessionSelected = len(m.sessions) - 1
		}
	}
}

func (m *Model) clampSelections() {
	if len(m.targets) == 0 {
		m.targetSelected = 0
	} else if m.targetSelected >= len(m.targets) {
		m.targetSelected = len(m.targets) - 1
	}

	if len(m.sessions) == 0 {
		m.sessionSelected = 0
	} else if m.sessionSelected >= len(m.sessions) {
		m.sessionSelected = len(m.sessions) - 1
	}
}

func (m *Model) syncLogs(force bool) {
	key, ok := m.currentLogKey()
	if !ok {
		m.closeLogSubscription()
		m.logKey = ""
		m.logBuffer = nil
		return
	}

	if !force && !m.logFollow && m.logKey == key {
		return
	}

	m.logKey = key
	if m.manager == nil {
		m.closeLogSubscription()
		m.logBuffer = nil
		return
	}

	lines, err := m.manager.LastLogs(key, m.logLines)
	if err != nil {
		m.closeLogSubscription()
		m.logBuffer = nil
		m.statusLevel = statusError
		m.status = fmt.Sprintf("%s: failed to load logs: %v", key, err)
		return
	}
	m.logBuffer = lines

	if !m.logFollow {
		m.closeLogSubscription()
		return
	}

	if m.logSubID != 0 && m.logSubKey == key && m.logSubCh != nil {
		return
	}

	m.closeLogSubscription()

	subID, ch, err := m.manager.SubscribeLogs(key, 64)
	if err != nil {
		m.logSubKey = ""
		m.logSubID = 0
		m.logSubCh = nil
		m.statusLevel = statusError
		m.status = fmt.Sprintf("%s: failed to follow logs: %v", key, err)
		return
	}

	m.logSubKey = key
	m.logSubID = subID
	m.logSubCh = ch
}

func (m Model) connectSelectedCmd() tea.Cmd {
	if m.manager == nil || len(m.targets) == 0 {
		return nil
	}

	target := m.targets[m.targetSelected]
	envCfg, err := findEnvConfig(m.cfg, target.Service, target.Env)
	if err != nil {
		return func() tea.Msg {
			return connectResultMsg{key: target.Key, err: err}
		}
	}

	opts := session.StartOptions{
		Service:          target.Service,
		Env:              target.Env,
		Bind:             m.defaults.Bind,
		TargetInstanceID: envCfg.TargetInstanceID,
		RemoteHost:       envCfg.RemoteHost,
		RemotePort:       envCfg.RemotePort,
		Region:           m.defaults.Region,
		Profile:          m.defaults.Profile,
		StartupTimeout:   time.Duration(m.defaults.StartupTimeoutSeconds) * time.Second,
	}
	if envCfg.LocalPort > 0 {
		opts.LocalPort = envCfg.LocalPort
	}
	if len(m.defaults.PortRange) == 2 {
		opts.PortMin = m.defaults.PortRange[0]
		opts.PortMax = m.defaults.PortRange[1]
	}

	return func() tea.Msg {
		s, err := m.manager.Start(opts)
		if err != nil {
			return connectResultMsg{key: target.Key, err: err}
		}
		return connectResultMsg{
			key:      target.Key,
			endpoint: fmt.Sprintf("%s:%d", s.Bind, s.LocalPort),
		}
	}
}

func (m Model) stopSelectedCmd() tea.Cmd {
	if m.manager == nil || len(m.sessions) == 0 {
		return nil
	}

	key := m.sessions[m.sessionSelected].Key
	return func() tea.Msg {
		return stopResultMsg{
			key: key,
			err: m.manager.Stop(key),
		}
	}
}

func (m Model) stopAllCmd() tea.Cmd {
	return func() tea.Msg {
		return stopAllResultMsg{err: m.manager.StopAll()}
	}
}

func (m Model) currentTargetKey() session.SessionKey {
	if len(m.targets) == 0 {
		return ""
	}
	return m.targets[m.targetSelected].Key
}

func (m Model) currentSessionKey() session.SessionKey {
	if len(m.sessions) == 0 {
		return ""
	}
	return m.sessions[m.sessionSelected].Key
}

func (m Model) currentLogKey() (session.SessionKey, bool) {
	switch m.focused {
	case PaneTargets:
		if len(m.targets) == 0 {
			return "", false
		}
		return m.targets[m.targetSelected].Key, true
	case PaneSessions:
		if len(m.sessions) == 0 {
			return "", false
		}
		return m.sessions[m.sessionSelected].Key, true
	case PaneLogs:
		if len(m.sessions) > 0 {
			return m.sessions[m.sessionSelected].Key, true
		}
		if len(m.targets) > 0 {
			return m.targets[m.targetSelected].Key, true
		}
	}
	return "", false
}

func (m *Model) closeLogSubscription() {
	if m.logSubID != 0 && m.manager != nil {
		m.manager.UnsubscribeLogs(m.logSubKey, m.logSubID)
	}
	m.logSubKey = ""
	m.logSubID = 0
	m.logSubCh = nil
	m.logReadActive = false
}

func (m Model) logReadCmd(key session.SessionKey, subID uint64, ch <-chan string) tea.Cmd {
	if ch == nil || subID == 0 {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return logLineMsg{key: key, subID: subID, closed: true}
		}
		return logLineMsg{key: key, subID: subID, line: line}
	}
}

func (m *Model) ensureLogReaderCmd() tea.Cmd {
	if m.logSubID == 0 || m.logSubCh == nil || m.logReadActive {
		return nil
	}
	m.logReadActive = true
	return m.logReadCmd(m.logSubKey, m.logSubID, m.logSubCh)
}

func findEnvConfig(cfg *config.Config, serviceName, envName string) (config.EnvConfig, error) {
	if cfg == nil {
		return config.EnvConfig{}, fmt.Errorf("%s/%s: config not loaded", serviceName, envName)
	}
	for _, svc := range cfg.Services {
		if svc.Name != serviceName {
			continue
		}
		envCfg, ok := svc.Envs[envName]
		if !ok {
			return config.EnvConfig{}, fmt.Errorf("%s/%s: environment not found in config", serviceName, envName)
		}
		return envCfg, nil
	}
	return config.EnvConfig{}, fmt.Errorf("%s/%s: service not found in config", serviceName, envName)
}

func configuredTargets(cfg *config.Config) []Target {
	if cfg == nil {
		return nil
	}

	targets := make([]Target, 0, len(cfg.Services))
	for _, svc := range cfg.Services {
		for envName := range svc.Envs {
			targets = append(targets, Target{
				Service: svc.Name,
				Env:     envName,
				Key:     session.NewSessionKey(svc.Name, envName),
			})
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Key < targets[j].Key
	})

	return targets
}
