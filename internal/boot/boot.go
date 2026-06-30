// Package boot wires reboot persistence on macOS via launchd.
//
// tmux survives disconnects but NOT a reboot. These LaunchAgents close the gap:
//   - ai.respawn.restore  : runs `respawn restore` at login (RunAtLoad)
//   - ai.respawn.snapshot : runs `respawn snapshot` periodically so the last
//     known session ids are always fresh on disk
//
// On Linux the equivalent is a systemd user service with `enable-linger`; see
// the README. Only the macOS path is wired up here.
package boot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	labelRestore     = "ai.respawn.restore"
	labelSnapshot    = "ai.respawn.snapshot"
	snapshotInterval = 300 // seconds
	// A login shell's PATH isn't guaranteed under launchd; cover the usual
	// homes of tmux and the respawn binary.
	bootPATH = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
)

func agentsDir() string {
	home, _ := os.UserHomeDir()
	d := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(d, 0o755)
	return d
}

func selfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "respawn"
	}
	return exe
}

func plist(label string, args []string, runAtLoad bool, interval int) string {
	home, _ := os.UserHomeDir()
	log := filepath.Join(home, "Library", "Logs", label+".log")
	var prog strings.Builder
	for _, a := range args {
		fmt.Fprintf(&prog, "      <string>%s</string>\n", a)
	}
	extra := ""
	if runAtLoad {
		extra += "    <key>RunAtLoad</key>\n    <true/>\n"
	}
	if interval > 0 {
		extra += fmt.Sprintf("    <key>StartInterval</key>\n    <integer>%d</integer>\n", interval)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
%s    </array>
    <key>EnvironmentVariables</key>
    <dict>
      <key>PATH</key>
      <string>%s</string>
    </dict>
%s    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
  </dict>
</plist>
`, label, prog.String(), bootPATH, extra, log, log)
}

// Install writes and loads the launchd agents. Returns the written paths.
func Install() ([]string, error) {
	self := selfPath()
	specs := []struct {
		label     string
		args      []string
		runAtLoad bool
		interval  int
	}{
		{labelRestore, []string{self, "restore"}, true, 0},
		{labelSnapshot, []string{self, "snapshot"}, false, snapshotInterval},
	}
	var written []string
	for _, s := range specs {
		path := filepath.Join(agentsDir(), s.label+".plist")
		if err := os.WriteFile(path, []byte(plist(s.label, s.args, s.runAtLoad, s.interval)), 0o644); err != nil {
			return written, err
		}
		exec.Command("launchctl", "unload", path).Run()
		exec.Command("launchctl", "load", path).Run()
		written = append(written, path)
	}
	return written, nil
}

// Uninstall unloads and removes the launchd agents. Returns removed paths.
func Uninstall() ([]string, error) {
	var removed []string
	for _, label := range []string{labelRestore, labelSnapshot} {
		path := filepath.Join(agentsDir(), label+".plist")
		if _, err := os.Stat(path); err == nil {
			exec.Command("launchctl", "unload", path).Run()
			if err := os.Remove(path); err != nil {
				return removed, err
			}
			removed = append(removed, path)
		}
	}
	return removed, nil
}
