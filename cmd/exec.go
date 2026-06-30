package cmd

import (
	"os/exec"
	"strings"
)

// gitRepoRoot returns the top-level directory of the git repo containing the
// current working directory, if any.
func gitRepoRoot() (string, bool) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", false
	}
	return root, true
}

// exec_LookPath resolves a binary in PATH (wrapper kept local to avoid a bare
// os/exec import collision with the syscall.Exec call site).
func exec_LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// runQuiet runs a command, ignoring output and errors (best-effort, e.g.
// focusing a window before attaching).
func runQuiet(name string, args ...string) {
	_ = exec.Command(name, args...).Run()
}
