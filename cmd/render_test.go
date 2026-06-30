package cmd

import (
	"testing"

	"github.com/biswajitpatra/respawn/internal/config"
	"github.com/biswajitpatra/respawn/internal/state"
)

func TestRenderBuiltinsAndArgs(t *testing.T) {
	job := state.Job{
		Name:      "frontend",
		Dir:       "/work/app",
		SessionID: "abc",
		Flags:     "--verbose",
		Args:      map[string]string{"port": "3000"},
	}
	got, err := render("run {name} --port {port} {flags} @{session_id} in {dir}", job)
	if err != nil {
		t.Fatal(err)
	}
	want := "run frontend --port 3000 --verbose @abc in /work/app"
	if got != want {
		t.Fatalf("render = %q, want %q", got, want)
	}
}

func TestRenderUnknownPlaceholder(t *testing.T) {
	_, err := render("run {nope}", state.Job{Name: "x"})
	if err == nil {
		t.Fatal("expected error for unknown placeholder")
	}
}

func TestBuildCommandResumeFallback(t *testing.T) {
	spec := config.ToolSpec{
		Start:          "claude",
		Resume:         "claude --resume {session_id}",
		ResumeFallback: "claude --continue",
	}
	// resume requested but no session id -> fallback
	got, err := buildCommand(state.Job{Name: "a"}, spec, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "claude --continue" {
		t.Fatalf("fallback = %q", got)
	}
	// resume with id -> resume template
	got, _ = buildCommand(state.Job{Name: "a", SessionID: "id1"}, spec, true)
	if got != "claude --resume id1" {
		t.Fatalf("resume = %q", got)
	}
}

func TestEnvPrefixQuotingAndOrder(t *testing.T) {
	job := state.Job{Env: map[string]string{"B": "two words", "A": "1"}}
	got := envPrefix(job)
	want := `A='1' B='two words'`
	if got != want {
		t.Fatalf("envPrefix = %q, want %q", got, want)
	}
}
