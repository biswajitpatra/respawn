// Package tmuxconf installs an optional, reversible tmux tweak: a window picker
// (prefix+w) sorted by last activity, scoped to respawn's own session so it
// never changes behavior in your other tmux sessions.
//
// It keeps the actual bindings in a respawn-owned snippet and adds only a single
// marker-delimited `source-file` line to your tmux config, so uninstall removes
// exactly what was added.
package tmuxconf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	beginMarker = "# >>> respawn (managed) >>>"
	endMarker   = "# <<< respawn (managed) <<<"
)

func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// SnippetPath is the respawn-owned tmux snippet.
func SnippetPath() string {
	return filepath.Join(configHome(), "respawn", "respawn.tmux")
}

// ConfPath finds the tmux config to edit: an explicit override, else an existing
// ~/.tmux.conf or XDG tmux.conf, else defaulting to ~/.tmux.conf. It never
// prefers a fresh ~/.tmux.conf over an existing XDG one (which tmux would then
// ignore).
func ConfPath() string {
	if v := os.Getenv("RESPAWN_TMUX_CONF"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	classic := filepath.Join(home, ".tmux.conf")
	xdg := filepath.Join(configHome(), "tmux", "tmux.conf")
	if _, err := os.Stat(classic); err == nil {
		return classic
	}
	if _, err := os.Stat(xdg); err == nil {
		return xdg
	}
	return classic
}

// Snippet is the tmux config respawn manages, parameterized by the session name.
func Snippet(session string) string {
	return fmt.Sprintf(`# respawn — managed snippet (edit via `+"`respawn install-tmux`"+`).
# prefix+w opens the window picker sorted by last activity, but ONLY in the
# respawn-managed session; every other session keeps tmux's default.
bind w if-shell '[ "#{session_name}" = "%s" ]' \
  'choose-tree -Zw -O time' \
  'choose-tree -Zw'
`, session)
}

func managedBlock() string {
	snip := SnippetPath()
	return fmt.Sprintf("%s\nif-shell '[ -f \"%s\" ]' 'source-file \"%s\"'\n%s\n",
		beginMarker, snip, snip, endMarker)
}

// stripBlock removes an existing managed block (and trailing blank line) from s.
func stripBlock(s string) string {
	start := strings.Index(s, beginMarker)
	if start == -1 {
		return s
	}
	end := strings.Index(s, endMarker)
	if end == -1 || end < start {
		return s
	}
	end += len(endMarker)
	// consume one trailing newline and any leading newline we added
	for end < len(s) && s[end] == '\n' {
		end++
	}
	before := strings.TrimRight(s[:start], "\n")
	after := s[end:]
	if before == "" {
		return after
	}
	if after == "" {
		return before + "\n"
	}
	return before + "\n\n" + after
}

// Install writes the snippet and adds/refreshes the managed block in the config.
func Install(session string) (conf, snippet string, err error) {
	snippet = SnippetPath()
	if err = os.MkdirAll(filepath.Dir(snippet), 0o755); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(snippet, []byte(Snippet(session)), 0o644); err != nil {
		return "", "", err
	}

	conf = ConfPath()
	if err = os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		return "", "", err
	}
	existing := ""
	if data, e := os.ReadFile(conf); e == nil {
		existing = string(data)
	}
	base := stripBlock(existing) // idempotent: drop any old block first
	if base != "" && !strings.HasSuffix(base, "\n") {
		base += "\n"
	}
	out := base + managedBlock()
	if err = os.WriteFile(conf, []byte(out), 0o644); err != nil {
		return "", "", err
	}
	return conf, snippet, nil
}

// Uninstall removes the managed block and the snippet file.
func Uninstall() (conf string, removedSnippet bool, err error) {
	conf = ConfPath()
	if data, e := os.ReadFile(conf); e == nil {
		stripped := stripBlock(string(data))
		if stripped != string(data) {
			if err = os.WriteFile(conf, []byte(stripped), 0o644); err != nil {
				return conf, false, err
			}
		}
	}
	if e := os.Remove(SnippetPath()); e == nil {
		removedSnippet = true
	}
	return conf, removedSnippet, nil
}
