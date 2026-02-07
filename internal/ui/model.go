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

const (
	PaneTargets  Pane = "targets"
	PaneSessions Pane = "sessions"
	PaneLogs     Pane = "logs"
)

type Target struct {
	Service string
	Env     string
	Key     session.SessionKey
}

type refreshTickMsg struct {
	sessions []session.SessionSummary
}

type Model struct {
	width  int
	height int

	targets   []Target
	sessions  []session.SessionSummary
	selected  int
	focused   Pane
	status    string
	manager   *session.Manager
	refreshIn time.Duration
}

func NewModel(manager *session.Manager, cfg *config.Config) Model {
	targets := configuredTargets(cfg)

	status := "ready"
	if len(targets) == 0 {
		status = "no configured targets found"
	}

	return Model{
		targets:   targets,
		focused:   PaneTargets,
		status:    status,
		manager:   manager,
		refreshIn: defaultRefreshInterval,
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
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case refreshTickMsg:
		m.sessions = msg.sessions
		if m.selected >= len(m.targets) && len(m.targets) > 0 {
			m.selected = len(m.targets) - 1
		}
		if len(m.targets) == 0 {
			m.selected = 0
		}
		m.status = fmt.Sprintf("targets=%d running=%d", len(m.targets), len(m.sessions))
		return m, m.refreshCmd()
	}

	return m, nil
}

func (m Model) View() string {
	return RenderView(m)
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		if m.refreshIn > 0 {
			time.Sleep(m.refreshIn)
		}
		if m.manager == nil {
			return refreshTickMsg{}
		}
		return refreshTickMsg{sessions: m.manager.List()}
	}
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
