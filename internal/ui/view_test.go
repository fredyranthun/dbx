package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fredyranthun/db/internal/session"
)

func makeTargets(total int) []Target {
	targets := make([]Target, 0, total)
	for i := 0; i < total; i++ {
		service := "service"
		env := fmt.Sprintf("env%02d", i)
		targets = append(targets, Target{
			Service: service,
			Env:     env,
			Key:     session.NewSessionKey(service, env),
		})
	}
	return targets
}

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

func TestRenderViewTargetsViewportShowsIndicators(t *testing.T) {
	m := Model{
		width:          120,
		height:         20,
		focused:        PaneTargets,
		targets:        makeTargets(15),
		targetSelected: 10,
		status:         "ok",
	}
	m.syncTargetViewport()

	out := RenderView(m)

	for _, want := range []string{"↑ 2 more", "↓ 4 more", "service/env10"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "service/env00") {
		t.Fatalf("expected output not to contain offscreen target service/env00\n%s", out)
	}
}
