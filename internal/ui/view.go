package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/fredyranthun/db/internal/session"
)

const (
	defaultWidth           = 120
	minWidth               = 72
	narrowLayoutBreakpoint = 110
)

var (
	appTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	summaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	paneBaseStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	paneFocusStyle = paneBaseStyle.Copy().BorderForeground(lipgloss.Color("39"))

	paneTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("254"))
	paneTitleMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectionStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	helpKeyStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)

	statusInfoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("31")).Padding(0, 1)
	statusOKStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("28")).Padding(0, 1)
	statusWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("232")).Background(lipgloss.Color("214")).Padding(0, 1)
	statusErrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("160")).Padding(0, 1)
)

func RenderView(m Model) string {
	width := m.width
	if width <= 0 {
		width = defaultWidth
	}
	if width < minWidth {
		width = minWidth
	}

	height := m.height
	if height <= 0 {
		height = 32
	}

	header := renderHeader(m, width)
	body := renderBody(m, width, height)
	status := renderStatusBar(m, width)
	help := renderHelpBar(width)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, status, help)
}

func renderHeader(m Model, width int) string {
	title := appTitleStyle.Render("dbx ui")
	summary := summaryStyle.Render(fmt.Sprintf("focus=%s  targets=%d  running=%d  follow=%t", m.focused, len(m.targets), runningCount(m.sessions), m.logFollow))

	content := lipgloss.JoinVertical(lipgloss.Left, title, summary)
	return lipgloss.NewStyle().Width(width).Padding(0, 0, 1, 0).Render(content)
}

func renderBody(m Model, width, height int) string {
	narrow := width < narrowLayoutBreakpoint
	if narrow {
		paneWidth := width
		return lipgloss.JoinVertical(
			lipgloss.Left,
			renderTargetsPane(m, paneWidth),
			renderSessionsPane(m, paneWidth),
			renderLogsPane(m, paneWidth, max(6, height/3)),
		)
	}

	leftWidth := (width - 1) / 2
	rightWidth := width - leftWidth - 1
	top := lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderTargetsPane(m, leftWidth),
		renderSessionsPane(m, rightWidth),
	)

	logsHeight := max(8, height-17)
	logs := renderLogsPane(m, width, logsHeight)

	return lipgloss.JoinVertical(lipgloss.Left, top, logs)
}

func renderTargetsPane(m Model, width int) string {
	title := paneTitle("targets", m.focused == PaneTargets, fmt.Sprintf("%d", len(m.targets)))
	lines := make([]string, 0, len(m.targets)+1)
	if len(m.targets) == 0 {
		lines = append(lines, mutedStyle.Render("No configured targets"))
	} else {
		for i, t := range m.targets {
			line := fmt.Sprintf("%s", t.Key)
			if i == m.targetSelected {
				line = selectionStyle.Render("› " + line)
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
		}
	}
	return renderPane(title, m.focused == PaneTargets, width, lines)
}

func renderSessionsPane(m Model, width int) string {
	title := paneTitle("sessions", m.focused == PaneSessions, fmt.Sprintf("running %d", runningCount(m.sessions)))
	lines := make([]string, 0, len(m.sessions)+2)
	if len(m.sessions) == 0 {
		lines = append(lines, mutedStyle.Render("No active sessions"))
	} else {
		head := mutedStyle.Render("KEY                      STATE      ENDPOINT              UPTIME")
		lines = append(lines, head)
		for i, s := range m.sessions {
			row := fmt.Sprintf("%-24s %-10s %-21s %s", s.Key, stateBadge(s.State), fmt.Sprintf("%s:%d", s.Bind, s.LocalPort), formatDuration(s.Uptime))
			if i == m.sessionSelected {
				row = selectionStyle.Render("› " + row)
			} else {
				row = "  " + row
			}
			lines = append(lines, row)
		}
	}
	return renderPane(title, m.focused == PaneSessions, width, lines)
}

func renderLogsPane(m Model, width, maxLines int) string {
	followLabel := "off"
	if m.logFollow {
		followLabel = "on"
	}
	sessionLabel := "(none)"
	if m.logKey != "" {
		sessionLabel = string(m.logKey)
	}
	title := paneTitle("logs", m.focused == PaneLogs, fmt.Sprintf("%s | follow %s", sessionLabel, followLabel))

	lines := make([]string, 0, maxLines)
	if len(m.logBuffer) == 0 {
		lines = append(lines, mutedStyle.Render("No logs for selected session yet"))
	} else {
		start := 0
		if len(m.logBuffer) > maxLines {
			start = len(m.logBuffer) - maxLines
		}
		for _, line := range m.logBuffer[start:] {
			lines = append(lines, line)
		}
	}

	return renderPane(title, m.focused == PaneLogs, width, lines)
}

func renderPane(title string, focused bool, width int, lines []string) string {
	if width < 24 {
		width = 24
	}
	style := paneBaseStyle
	if focused {
		style = paneFocusStyle
	}

	innerWidth := max(1, width-4)
	body := make([]string, 0, len(lines)+1)
	body = append(body, title)
	for _, line := range lines {
		body = append(body, truncate(line, innerWidth))
	}

	return style.Width(width).Render(strings.Join(body, "\n"))
}

func paneTitle(name string, focused bool, right string) string {
	left := strings.ToUpper(name)
	if focused {
		left += " *"
	}
	if right == "" {
		return paneTitleStyle.Render(left)
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		paneTitleStyle.Render(left),
		paneTitleMutedStyle.Render("  "+right),
	)
}

func renderStatusBar(m Model, width int) string {
	msg := strings.TrimSpace(m.status)
	if msg == "" {
		msg = "ready"
	}

	style := statusInfoStyle
	switch m.statusLevel {
	case statusSuccess:
		style = statusOKStyle
	case statusWarn:
		style = statusWarnStyle
	case statusError:
		style = statusErrStyle
	}

	return style.Width(width).Render("status: " + msg)
}

func renderHelpBar(width int) string {
	parts := []string{
		helpKeyStyle.Render("j/k") + " move",
		helpKeyStyle.Render("tab") + " focus",
		helpKeyStyle.Render("c") + " connect",
		helpKeyStyle.Render("s") + " stop",
		helpKeyStyle.Render("S") + " stop-all",
		helpKeyStyle.Render("l") + " follow",
		helpKeyStyle.Render("q") + " quit",
	}
	line := strings.Join(parts, "  ")
	return lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color("246")).Render(line)
}

func truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	if width <= 1 {
		return string(runes[:1])
	}
	if len(runes) > width-1 {
		runes = runes[:width-1]
	}
	return string(runes) + "…"
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.Truncate(time.Second).String()
}

func stateBadge(state session.SessionState) string {
	text := string(state)
	switch state {
	case session.SessionStateRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("41")).Render(text)
	case session.SessionStateStarting:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(text)
	case session.SessionStateError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(text)
	default:
		return mutedStyle.Render(text)
	}
}

func runningCount(sessions []session.SessionSummary) int {
	count := 0
	for _, s := range sessions {
		if s.State == session.SessionStateRunning {
			count++
		}
	}
	return count
}
