package capture

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/biswajitpatra/respawn/internal/config"
)

func TestClaudeProjectDir(t *testing.T) {
	got := claudeProjectDir("/Users/me/work_app.v2")
	want := "-Users-me-work-app-v2"
	if got != want {
		t.Fatalf("claudeProjectDir = %q, want %q", got, want)
	}
}

func TestArgID(t *testing.T) {
	c := config.Capture{Kind: "arg", Pattern: `--resume\s+(\S+)`}
	if got := SessionID(c, "/tmp", "claude --resume abc123 --model opus"); got != "abc123" {
		t.Fatalf("arg capture = %q, want abc123", got)
	}
	if got := SessionID(c, "/tmp", "claude --model opus"); got != "" {
		t.Fatalf("arg capture with no match = %q, want empty", got)
	}
}

func TestNewestFileID(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.jsonl")
	newer := filepath.Join(dir, "newer.jsonl")
	for _, f := range []string{old, newer} {
		if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Make `old` older than `newer`.
	past := time.Now().Add(-time.Hour)
	os.Chtimes(old, past, past)

	c := config.Capture{Kind: "newest_file", Base: dir, Project: "none", Glob: "*.jsonl"}
	if got := SessionID(c, "/whatever", ""); got != "newer" {
		t.Fatalf("newest_file capture = %q, want 'newer'", got)
	}
}

func TestNoneCapture(t *testing.T) {
	if got := SessionID(config.Capture{Kind: "none"}, "/tmp", "anything"); got != "" {
		t.Fatalf("none capture = %q, want empty", got)
	}
}
