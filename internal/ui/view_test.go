package ui

import (
	"strings"
	"testing"

	"github.com/fredyranthun/db/internal/session"
)

func TestRenderViewIncludesCoreSections(t *testing.T) {
	m := Model{
		focused: PaneTargets,
		targets: []Target{{Key: session.NewSessionKey("service1", "dev")}},
		sessions: []session.SessionSummary{{
			Key:       session.NewSessionKey("service1", "dev"),
			Bind:      "127.0.0.1",
			LocalPort: 5500,
			State:     session.SessionStateRunning,
		}},
		logBuffer: []string{"line-1"},
		status:    "ok",
	}

	out := RenderView(m)
	for _, want := range []string{"dbx ui", "[targets*]", "[sessions]", "[logs]", "status: ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\n%s", want, out)
		}
	}
}
