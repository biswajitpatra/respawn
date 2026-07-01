package tmuxconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUninstall(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "tmux.conf")
	t.Setenv("RESPAWN_TMUX_CONF", conf)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	if err := os.WriteFile(conf, []byte("set -g mouse on\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, snip, err := Install("respawn")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(conf)
	s := string(got)
	if !strings.Contains(s, "set -g mouse on") {
		t.Fatal("clobbered pre-existing user config")
	}
	if !strings.Contains(s, beginMarker) || !strings.Contains(s, "source-file") {
		t.Fatalf("managed block missing:\n%s", s)
	}
	sb, err := os.ReadFile(snip)
	if err != nil {
		t.Fatalf("snippet not written: %v", err)
	}
	if !strings.Contains(string(sb), "session_name") || !strings.Contains(string(sb), "choose-tree -Zw -O time") {
		t.Fatalf("snippet missing session-scoped binding:\n%s", sb)
	}

	// Idempotent: installing again must not duplicate the block.
	if _, _, err := Install("respawn"); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(conf)
	if n := strings.Count(string(again), beginMarker); n != 1 {
		t.Fatalf("expected 1 managed block, got %d", n)
	}

	// Uninstall removes the block + snippet, keeps user config.
	if _, _, err := Uninstall(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(conf)
	if strings.Contains(string(after), beginMarker) {
		t.Fatal("managed block not removed")
	}
	if !strings.Contains(string(after), "set -g mouse on") {
		t.Fatal("user config lost on uninstall")
	}
	if _, err := os.Stat(snip); !os.IsNotExist(err) {
		t.Fatal("snippet not removed on uninstall")
	}
}
