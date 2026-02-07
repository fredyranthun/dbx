package ui

import "fmt"

func RenderView(m Model) string {
	selected := "-"
	if len(m.targets) > 0 && m.selected >= 0 && m.selected < len(m.targets) {
		selected = m.targets[m.selected].Key.String()
	}

	return fmt.Sprintf(
		"dbx ui (placeholder)\n\n"+
			"Press q or ctrl+c to quit.\n\n"+
			"Focus: %s\n"+
			"Selected: %s\n"+
			"Configured targets: %d\n"+
			"Running sessions: %d\n"+
			"Status: %s\n\n"+
			"Size: %dx%d\n",
		m.focused,
		selected,
		len(m.targets),
		len(m.sessions),
		m.status,
		m.width,
		m.height,
	)
}
