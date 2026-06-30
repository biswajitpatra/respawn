package cmd

import "os/exec"

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
