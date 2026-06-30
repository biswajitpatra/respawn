// Package capture pulls a resumable session id out of a running (or past) tool.
//
// Strategies, selected per tool in the registry:
//   - newest_file : the tool writes a transcript per session; the newest file's
//     stem IS the id (e.g. Claude Code's
//     ~/.claude/projects/<proj>/<id>.jsonl). Works even after the
//     process has exited — it reads disk.
//   - arg         : the id is on the command line (e.g. `--resume <id>`).
//   - none        : no resumable id; RESUME falls back to START.
package capture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/biswajitpatra/respawn/internal/config"
)

func expand(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

var nonDash = regexp.MustCompile(`[/_.]`)

// claudeProjectDir mirrors how Claude Code encodes a cwd into its projects
// folder name: path separators and a few other chars become '-'.
// e.g. /Users/me/work_app -> -Users-me-work-app
func claudeProjectDir(workDir string) string {
	abs, err := filepath.Abs(expand(workDir))
	if err != nil {
		abs = workDir
	}
	return nonDash.ReplaceAllString(abs, "-")
}

func newestFileID(c config.Capture, workDir string) string {
	base := expand(c.Base)
	var folder string
	switch c.Project {
	case "claude":
		folder = filepath.Join(base, claudeProjectDir(workDir))
	case "basename":
		folder = filepath.Join(base, filepath.Base(workDir))
	default:
		folder = base
	}
	glob := c.Glob
	if glob == "" {
		glob = "*"
	}
	matches, err := filepath.Glob(filepath.Join(folder, glob))
	if err != nil || len(matches) == 0 {
		return ""
	}
	var newest string
	var newestMod int64 = -1
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil || fi.IsDir() {
			continue
		}
		if fi.ModTime().UnixNano() > newestMod {
			newestMod = fi.ModTime().UnixNano()
			newest = m
		}
	}
	if newest == "" {
		return ""
	}
	name := filepath.Base(newest)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func argID(c config.Capture, cmdline string) string {
	if cmdline == "" || c.Pattern == "" {
		return ""
	}
	re, err := regexp.Compile(c.Pattern)
	if err != nil {
		return ""
	}
	m := re.FindStringSubmatch(cmdline)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// SessionID returns the captured id, or "" if none could be found.
//
// kind="assign" returns "" here on purpose: that id is generated and pinned at
// `add` time (see cmd.assignSessionID), so snapshot must not try to re-derive
// or overwrite it.
func SessionID(c config.Capture, workDir, cmdline string) string {
	switch c.Kind {
	case "newest_file":
		return newestFileID(c, workDir)
	case "arg":
		return argID(c, cmdline)
	default:
		return ""
	}
}
