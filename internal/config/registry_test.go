package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Point user config at an empty dir so only defaults load.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	reg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	claude, ok := reg["claude"]
	if !ok {
		t.Fatal("expected built-in 'claude' tool")
	}
	if claude.Detect != "claude" {
		t.Fatalf("claude.Detect = %q", claude.Detect)
	}
	if claude.Capture.Kind != "newest_file" {
		t.Fatalf("claude.Capture.Kind = %q, want newest_file", claude.Capture.Kind)
	}
}

func TestUserOverrideMergesFieldwise(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	dir := filepath.Join(cfg, "respawn")
	os.MkdirAll(dir, 0o755)
	// Override only claude's start; everything else should remain from defaults.
	override := `[tools.claude]
start = "claude --custom"
[tools.mytool]
detect = "mybin"
start = "mybin run"
`
	if err := os.WriteFile(filepath.Join(dir, "tools.toml"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if reg["claude"].Start != "claude --custom" {
		t.Fatalf("override start not applied: %q", reg["claude"].Start)
	}
	if reg["claude"].Resume == "" {
		t.Fatal("override clobbered claude.Resume; expected fieldwise merge")
	}
	if reg["mytool"].Detect != "mybin" {
		t.Fatal("user-defined tool not added")
	}
}
