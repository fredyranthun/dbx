package ui

import (
	"fmt"
	"strings"
	"time"
)

func RenderView(m Model) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	if width < 80 {
		width = 80
	}

	leftWidth := (width - 3) / 2
	rightWidth := width - leftWidth - 3

	var b strings.Builder
	b.WriteString("dbx ui\n")
	fmt.Fprintf(&b, "focus=%s targets=%d running=%d follow=%t\n", m.focused, len(m.targets), len(m.sessions), m.logFollow)
	b.WriteString("keys: j/k/up/down move | tab focus | c connect | s stop | S stop-all | l follow | q quit\n\n")

	left := renderTargetsPane(m, leftWidth)
	right := renderSessionsPane(m, rightWidth)
	b.WriteString(joinColumns(left, right, leftWidth, rightWidth))
	b.WriteString("\n\n")
	b.WriteString(strings.Join(renderLogsPane(m, width), "\n"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "status: %s\n", m.status)

	return b.String()
}

func renderTargetsPane(m Model, width int) []string {
	lines := []string{paneTitle("targets", m.focused == PaneTargets)}
	if len(m.targets) == 0 {
		return append(lines, "(none)")
	}
	for i, t := range m.targets {
		indicator := " "
		if i == m.targetSelected {
			indicator = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s", indicator, t.Key), width))
	}
	return lines
}

func renderSessionsPane(m Model, width int) []string {
	lines := []string{paneTitle("sessions", m.focused == PaneSessions)}
	if len(m.sessions) == 0 {
		return append(lines, "(none)")
	}
	for i, s := range m.sessions {
		indicator := " "
		if i == m.sessionSelected {
			indicator = ">"
		}
		lines = append(
			lines,
			truncate(
				fmt.Sprintf(
					"%s %s %-8s %s:%d %s",
					indicator,
					s.Key,
					s.State,
					s.Bind,
					s.LocalPort,
					formatDuration(s.Uptime),
				),
				width,
			),
		)
	}
	return lines
}

func renderLogsPane(m Model, width int) []string {
	lines := []string{paneTitle("logs", m.focused == PaneLogs)}
	if m.logKey != "" {
		lines = append(lines, truncate(fmt.Sprintf("session=%s", m.logKey), width))
	} else {
		lines = append(lines, "session=(none)")
	}

	if len(m.logBuffer) == 0 {
		return append(lines, "(no logs)")
	}

	maxLines := 12
	if m.height > 0 {
		maxLines = m.height - 12
		if maxLines < 5 {
			maxLines = 5
		}
	}

	start := 0
	if len(m.logBuffer) > maxLines {
		start = len(m.logBuffer) - maxLines
	}
	for _, line := range m.logBuffer[start:] {
		lines = append(lines, truncate(line, width))
	}
	return lines
}

func joinColumns(left, right []string, leftWidth, rightWidth int) string {
	count := len(left)
	if len(right) > count {
		count = len(right)
	}
	lines := make([]string, 0, count)
	for i := 0; i < count; i++ {
		l := ""
		r := ""
		if i < len(left) {
			l = truncate(left[i], leftWidth)
		}
		if i < len(right) {
			r = truncate(right[i], rightWidth)
		}
		lines = append(lines, fmt.Sprintf("%-*s   %-*s", leftWidth, l, rightWidth, r))
	}
	return strings.Join(lines, "\n")
}

func paneTitle(name string, focused bool) string {
	if focused {
		return fmt.Sprintf("[%s*]", name)
	}
	return fmt.Sprintf("[%s]", name)
}

func truncate(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.Truncate(time.Second).String()
}
