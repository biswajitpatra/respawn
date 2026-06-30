// Package proc walks the OS process tree to find a tool running under a tmux
// pane (so respawn can read its command line for session-id capture).
package proc

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type process struct {
	pid     int
	ppid    int
	command string
}

func snapshot() map[int]process {
	out, err := exec.Command("ps", "-ax", "-o", "pid=,ppid=,command=").Output()
	procs := map[int]process{}
	if err != nil {
		return procs
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		// command is everything after pid and ppid; keep its internal spacing.
		idx := indexOfNthField(line, 2)
		procs[pid] = process{pid: pid, ppid: ppid, command: strings.TrimSpace(line[idx:])}
	}
	return procs
}

// indexOfNthField returns the byte offset where the (0-based) nth whitespace-
// separated field begins, so we can keep the command's internal spacing.
func indexOfNthField(line string, n int) int {
	i, field := 0, 0
	inSpace := true
	for i < len(line) {
		c := line[i]
		isSpace := c == ' ' || c == '\t'
		if !isSpace && inSpace {
			if field == n {
				return i
			}
			field++
		}
		inSpace = isSpace
		i++
	}
	return len(line)
}

func matches(command, detect string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	first := fields[0]
	base := filepath.Base(first)
	return base == detect || strings.HasPrefix(base, detect+"-") || strings.Contains(first, "/"+detect)
}

// FindToolProc DFS-walks descendants of rootPID for a process whose command
// names `detect` (the tool binary). Returns its full command line.
func FindToolProc(rootPID int, detect string) (string, bool) {
	procs := snapshot()
	children := map[int][]int{}
	for _, p := range procs {
		children[p.ppid] = append(children[p.ppid], p.pid)
	}
	stack := append([]int{}, children[rootPID]...)
	seen := map[int]bool{}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		if p, ok := procs[pid]; ok {
			if matches(p.command, detect) {
				return p.command, true
			}
			stack = append(stack, children[pid]...)
		}
	}
	return "", false
}
