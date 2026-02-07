package ui

import "fmt"

func RenderView(m Model) string {
	return fmt.Sprintf(
		"dbx ui (placeholder)\n\n"+
			"Session manager TUI scaffolding is ready.\n"+
			"Press q or ctrl+c to quit.\n\n"+
			"Size: %dx%d\n",
		m.width,
		m.height,
	)
}
