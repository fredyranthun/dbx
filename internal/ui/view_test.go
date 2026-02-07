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
	for _, want := range []string{"dbx ui", "TARGETS", "SESSIONS", "LOGS", "status: ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\n%s", want, out)
		}
	}
}

func TestRenderViewRunningCountOnlyIncludesRunningState(t *testing.T) {
	m := Model{
		focused: PaneTargets,
		targets: []Target{{Key: session.NewSessionKey("service1", "dev")}},
		sessions: []session.SessionSummary{
			{Key: session.NewSessionKey("service1", "dev"), State: session.SessionStateStopped},
			{Key: session.NewSessionKey("service2", "dev"), State: session.SessionStateRunning},
		},
	}

	out := RenderView(m)
	if !strings.Contains(out, "running=1") {
		t.Fatalf("expected running count to include only running sessions, got:\n%s", out)
	}
}

func TestRenderViewNarrowLayoutStillShowsAllPanes(t *testing.T) {
	m := Model{
		width:   90,
		focused: PaneLogs,
		targets: []Target{{Key: session.NewSessionKey("service1", "dev")}},
		sessions: []session.SessionSummary{{
			Key:       session.NewSessionKey("service1", "dev"),
			Bind:      "127.0.0.1",
			LocalPort: 5500,
			State:     session.SessionStateRunning,
		}},
		logBuffer: []string{"line-1", "line-2"},
		status:    "ok",
	}

	out := RenderView(m)
	for _, want := range []string{"TARGETS", "SESSIONS", "LOGS", "line-2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q in narrow layout\n%s", want, out)
		}
	}
}
