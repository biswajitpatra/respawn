// Package tmux drives tmux through its command interface.
//
// tmux is a client-server program whose CLI *is* its API: each `tmux <cmd>`
// talks to the running server, and `-F '#{...}'` format strings return
// structured output. (This is exactly what libraries like libtmux do under the
// hood.) respawn keeps every managed job as a named window inside one tmux
// session so they live in one place, survive disconnects, and can be recreated
// wholesale on reboot.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Session is the tmux session respawn manages (override with RESPAWN_SESSION).
func Session() string {
	if s := os.Getenv("RESPAWN_SESSION"); s != "" {
		return s
	}
	return "respawn"
}

func target(window string) string { return Session() + ":" + window }

func run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// EnsureSession creates the managed session (detached) if it doesn't exist.
func EnsureSession() error {
	if err := exec.Command("tmux", "has-session", "-t", "="+Session()).Run(); err == nil {
		return nil
	}
	_, err := run("new-session", "-d", "-s", Session())
	return err
}

// HasWindow reports whether a window of this name exists in the session.
func HasWindow(name string) bool {
	out, err := run("list-windows", "-t", "="+Session(), "-F", "#{window_name}")
	if err != nil {
		return false
	}
	for _, w := range strings.Split(out, "\n") {
		if w == name {
			return true
		}
	}
	return false
}

// LaunchWindow creates (or reuses) the job's window and runs command in it.
func LaunchWindow(name, dir, command string) error {
	if err := EnsureSession(); err != nil {
		return err
	}
	if !HasWindow(name) {
		if _, err := run("new-window", "-d", "-t", Session()+":", "-n", name, "-c", dir); err != nil {
			return err
		}
	}
	// Type the command literally, then press Enter as a separate key event so a
	// command that happens to contain a key-name token isn't misinterpreted.
	if _, err := run("send-keys", "-t", target(name), "-l", command); err != nil {
		return err
	}
	_, err := run("send-keys", "-t", target(name), "Enter")
	return err
}

// KillWindow removes the job's window. Returns false if it didn't exist.
func KillWindow(name string) bool {
	if !HasWindow(name) {
		return false
	}
	_, err := run("kill-window", "-t", target(name))
	return err == nil
}

// WindowActivities returns each window's last-activity epoch (seconds) — a
// generic, tool-agnostic signal of when the job last produced output.
func WindowActivities() map[string]int64 {
	res := map[string]int64{}
	out, err := run("list-windows", "-t", "="+Session(), "-F", "#{window_name}\t#{window_activity}")
	if err != nil {
		return res
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
			res[parts[0]] = v
		}
	}
	return res
}

// PanePID returns the shell pid of the window's active pane.
func PanePID(name string) (int, bool) {
	out, err := run("display-message", "-p", "-t", target(name), "#{pane_pid}")
	if err != nil || out == "" {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, false
	}
	return pid, true
}
