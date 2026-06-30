// Package cmd implements the respawn CLI.
package cmd

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/biswajitpatra/respawn/internal/boot"
	"github.com/biswajitpatra/respawn/internal/capture"
	"github.com/biswajitpatra/respawn/internal/config"
	"github.com/biswajitpatra/respawn/internal/proc"
	"github.com/biswajitpatra/respawn/internal/state"
	"github.com/biswajitpatra/respawn/internal/tmux"
)

var rootCmd = &cobra.Command{
	Use:           "respawn",
	Short:         "Persist and resurrect long-running tmux sessions with per-tool start/resume templates.",
	Long:          "respawn keeps long-running, interactive processes (AI coding sessions, dev servers,\ntraining runs, bots) alive in one tmux session and resumes each one — where it\nleft off — after a reboot.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI.
func Execute(version string) {
	rootCmd.Version = resolveVersion(version)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// resolveVersion enriches a "dev" build with the embedded git revision so you
// can tell whether an installed binary matches the latest source.
func resolveVersion(v string) string {
	if v != "" && v != "dev" {
		return v
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	rev, dirty := "", ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				rev = s.Value[:7]
			} else {
				rev = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		return "dev+" + rev + dirty
	}
	return "dev"
}

func init() {
	rootCmd.AddCommand(addCmd, lsCmd, snapshotCmd, restoreCmd, restartCmd, stopCmd, rmCmd, attachCmd, toolsCmd, installBootCmd, uninstallBootCmd)
}

// --- shared rendering ---------------------------------------------------------

var placeholder = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

func render(tmpl string, job state.Job) (string, error) {
	vals := map[string]string{
		"name":       job.Name,
		"dir":        job.Dir,
		"session_id": job.SessionID,
		"flags":      job.Flags,
	}
	for k, v := range job.Args {
		vals[k] = v
	}
	var missing []string
	out := placeholder.ReplaceAllStringFunc(tmpl, func(m string) string {
		key := m[1 : len(m)-1]
		if v, ok := vals[key]; ok {
			return v
		}
		missing = append(missing, key)
		return m
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("template references unknown placeholder(s) %v; provide with `-a key=val` (built-ins: name, dir, session_id, flags)", missing)
	}
	return out, nil
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func envPrefix(job state.Job) string {
	keys := make([]string, 0, len(job.Env))
	for k := range job.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		if job.Env[k] != "" {
			parts = append(parts, k+"="+shellQuote(job.Env[k]))
		}
	}
	return strings.Join(parts, " ")
}

// newUUID returns a random RFC-4122 v4 UUID (no external dependency).
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// assignSessionID generates a session id to pin at launch for tools whose
// capture kind is "assign" (e.g. `claude --session-id <uuid>`). This makes each
// job's id deterministic and unique — the only reliable way to run several of
// the same tool in one directory. Returns "" for non-assign tools.
func assignSessionID(spec config.ToolSpec) string {
	if spec.Capture.Kind != "assign" {
		return ""
	}
	switch spec.Capture.Format {
	case "", "uuid":
		return newUUID()
	default:
		return newUUID()
	}
}

func buildCommand(job state.Job, spec config.ToolSpec, resume bool) (string, error) {
	var tmpl string
	switch {
	case resume && job.SessionID != "" && spec.Resume != "":
		tmpl = spec.Resume
	case resume && job.SessionID == "" && spec.ResumeFallback != "":
		tmpl = spec.ResumeFallback
	default:
		tmpl = spec.Start
	}
	body, err := render(tmpl, job)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(envPrefix(job) + " " + body), nil
}

func snapshotJob(job *state.Job, spec config.ToolSpec) string {
	cmdline := ""
	if tmux.HasWindow(job.Name) {
		if pid, ok := tmux.PanePID(job.Name); ok {
			if cl, found := proc.FindToolProc(pid, spec.Detect); found {
				cmdline = cl
			}
		}
	}
	if sid := capture.SessionID(spec.Capture, job.Dir, cmdline); sid != "" {
		job.SessionID = sid
	}
	return job.SessionID
}

func status(job state.Job, spec config.ToolSpec) string {
	if !tmux.HasWindow(job.Name) {
		return "down"
	}
	if pid, ok := tmux.PanePID(job.Name); ok {
		if _, found := proc.FindToolProc(pid, spec.Detect); found {
			return "running"
		}
	}
	return "idle"
}

func parseKV(items []string) map[string]string {
	out := map[string]string{}
	for _, it := range items {
		if i := strings.Index(it, "="); i > 0 {
			out[it[:i]] = it[i+1:]
		}
	}
	return out
}

// --- commands -----------------------------------------------------------------

var (
	addTool     string
	addDir      string
	addArgs     []string
	addEnv      []string
	addNoLaunch bool
)

var addCmd = &cobra.Command{
	Use:   "add NAME -t TOOL [-d DIR] [-a k=v ...] [-e K=V ...] [-- TOOL FLAGS]",
	Short: "Register a new job and start it in a tmux window",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		flags := ""
		if dash := cmd.ArgsLenAtDash(); dash >= 0 {
			flags = strings.Join(args[dash:], " ")
		}

		spec, err := config.Get(addTool)
		if err != nil {
			return err
		}
		jobs, err := state.Load()
		if err != nil {
			return err
		}
		if _, exists := jobs[name]; exists {
			return fmt.Errorf("job %q already exists (use `respawn restart %s`)", name, name)
		}

		workDir := resolveDir(addDir)
		userArgs := parseKV(addArgs)
		userEnv := parseKV(addEnv)

		job := state.Job{Name: name, Tool: addTool, Dir: workDir, Flags: flags, Args: userArgs}

		// Resolve env: an empty template captures the var from the current
		// environment; a non-empty template renders against the job ({name},
		// {dir}, named args, ...).
		resolvedEnv := map[string]string{}
		for key, tmpl := range spec.Env {
			var val string
			if tmpl == "" {
				val = os.Getenv(key)
			} else {
				rendered, err := render(tmpl, job)
				if err != nil {
					return err
				}
				val = rendered
			}
			if val != "" {
				resolvedEnv[key] = val
			}
		}
		for k, v := range userEnv { // explicit --env always wins
			resolvedEnv[k] = v
		}
		job.Env = resolvedEnv

		// Pin a session id at launch for tools that assign one (e.g. claude
		// --session-id), so multiple jobs of the same tool in one directory each
		// get a unique, deterministic id instead of racing over a shared folder.
		job.SessionID = assignSessionID(spec)

		// Render (and validate) the start command BEFORE persisting, so a bad
		// template doesn't leak a half-registered job into state.
		command, err := buildCommand(job, spec, false)
		if err != nil {
			return err
		}

		jobs[name] = job
		if err := state.Save(jobs); err != nil {
			return err
		}

		if addNoLaunch {
			fmt.Printf("registered %s (%s) in %s — not launched\n", name, addTool, workDir)
			return nil
		}
		if err := tmux.LaunchWindow(name, workDir, command); err != nil {
			return err
		}
		fmt.Printf("started %s: %s\n", name, command)
		return nil
	},
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List every managed job across all repos, with live status",
	RunE: func(cmd *cobra.Command, args []string) error {
		jobs, err := state.Load()
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			fmt.Println("no jobs yet — `respawn add <name> --tool claude --dir <path>`")
			return nil
		}
		reg, err := config.Load()
		if err != nil {
			return err
		}
		names := make([]string, 0, len(jobs))
		for n := range jobs {
			names = append(names, n)
		}
		sort.Strings(names)
		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTOOL\tSTATUS\tSESSION\tDIR")
		for _, n := range names {
			j := jobs[n]
			st := "?"
			if spec, ok := reg[j.Tool]; ok {
				st = status(j, spec)
			}
			sid := j.SessionID
			if sid == "" {
				sid = "-"
			} else if len(sid) > 12 {
				sid = sid[:12]
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", j.Name, j.Tool, st, sid, j.Dir)
		}
		return w.Flush()
	},
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Refresh every job's session id on disk (run on a timer for boot safety)",
	RunE: func(cmd *cobra.Command, args []string) error {
		jobs, err := state.Load()
		if err != nil {
			return err
		}
		reg, err := config.Load()
		if err != nil {
			return err
		}
		n := 0
		for name, j := range jobs {
			spec, ok := reg[j.Tool]
			if !ok {
				continue
			}
			if snapshotJob(&j, spec) != "" {
				n++
			}
			jobs[name] = j
		}
		if err := state.Save(jobs); err != nil {
			return err
		}
		fmt.Printf("snapshot: %d/%d job(s) have a session id\n", n, len(jobs))
		return nil
	},
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Recreate every job's window and resume it (run at boot)",
	RunE: func(cmd *cobra.Command, args []string) error {
		jobs, err := state.Load()
		if err != nil {
			return err
		}
		reg, err := config.Load()
		if err != nil {
			return err
		}
		names := make([]string, 0, len(jobs))
		for n := range jobs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			j := jobs[name]
			spec, ok := reg[j.Tool]
			if !ok {
				fmt.Printf("skip %s: unknown tool %q\n", j.Name, j.Tool)
				continue
			}
			command, err := buildCommand(j, spec, true)
			if err != nil {
				fmt.Printf("skip %s: %v\n", j.Name, err)
				continue
			}
			if err := tmux.LaunchWindow(j.Name, j.Dir, command); err != nil {
				fmt.Printf("skip %s: %v\n", j.Name, err)
				continue
			}
			fmt.Printf("restored %s: %s\n", j.Name, command)
		}
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart NAME",
	Short: "Capture the current session, kill the window, and resume the same one",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		jobs, err := state.Load()
		if err != nil {
			return err
		}
		job, ok := jobs[name]
		if !ok {
			return fmt.Errorf("no job %q", name)
		}
		spec, err := config.Get(job.Tool)
		if err != nil {
			return err
		}
		snapshotJob(&job, spec)
		jobs[name] = job
		if err := state.Save(jobs); err != nil {
			return err
		}
		tmux.KillWindow(name)
		command, err := buildCommand(job, spec, true)
		if err != nil {
			return err
		}
		if err := tmux.LaunchWindow(name, job.Dir, command); err != nil {
			return err
		}
		fmt.Printf("restarted %s: %s\n", name, command)
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop NAME",
	Short: "Stop a running job (kill its window) but keep it registered",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		jobs, err := state.Load()
		if err != nil {
			return err
		}
		job, ok := jobs[name]
		if !ok {
			return fmt.Errorf("no job %q", name)
		}
		// Capture the latest session id before killing so a later restart/
		// restore can resume where it left off.
		if spec, err := config.Get(job.Tool); err == nil {
			snapshotJob(&job, spec)
			jobs[name] = job
			_ = state.Save(jobs)
		}
		if tmux.KillWindow(name) {
			fmt.Printf("stopped %s (still registered — `respawn restart %s` to resume)\n", name, name)
		} else {
			fmt.Printf("%s was not running\n", name)
		}
		return nil
	},
}

var rmKill bool

var rmCmd = &cobra.Command{
	Use:   "rm NAME",
	Short: "Remove a job from the registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		jobs, err := state.Load()
		if err != nil {
			return err
		}
		if _, ok := jobs[name]; !ok {
			return fmt.Errorf("no job %q", name)
		}
		if rmKill {
			tmux.KillWindow(name)
		}
		delete(jobs, name)
		if err := state.Save(jobs); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", name)
		return nil
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach [NAME]",
	Short: "Attach to the respawn tmux session (optionally focusing one job)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := exec_LookPath("tmux")
		if err != nil {
			return err
		}
		sess := tmux.Session()
		if len(args) == 1 {
			runQuiet(path, "select-window", "-t", sess+":"+args[0])
		}
		var argv []string
		if os.Getenv("TMUX") != "" {
			argv = []string{"tmux", "switch-client", "-t", sess}
		} else {
			argv = []string{"tmux", "attach", "-t", sess}
		}
		return syscall.Exec(path, argv, os.Environ())
	},
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List the tool registry (defaults merged with your overrides)",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Printf("registry (override at %s):\n\n", config.UserConfigPath())
		names := make([]string, 0, len(reg))
		for n := range reg {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			spec := reg[n]
			resume := spec.Resume
			if resume == "" {
				resume = "(falls back to start)"
			}
			fmt.Printf("  %s\n    start  : %s\n    resume : %s\n    capture: %s\n", n, spec.Start, resume, spec.Capture.Kind)
		}
		return nil
	},
}

var installBootCmd = &cobra.Command{
	Use:   "install-boot",
	Short: "Install launchd agents so jobs restore at login (macOS)",
	RunE: func(cmd *cobra.Command, args []string) error {
		written, err := boot.Install()
		for _, p := range written {
			fmt.Printf("installed %s\n", p)
		}
		if err != nil {
			return err
		}
		fmt.Println("jobs will restore at next login; snapshots run every 5 min.")
		return nil
	},
}

var uninstallBootCmd = &cobra.Command{
	Use:   "uninstall-boot",
	Short: "Remove the launchd agents (macOS)",
	RunE: func(cmd *cobra.Command, args []string) error {
		removed, err := boot.Uninstall()
		for _, p := range removed {
			fmt.Printf("removed %s\n", p)
		}
		if err != nil {
			return err
		}
		if len(removed) == 0 {
			fmt.Println("nothing to remove")
		}
		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addTool, "tool", "t", "", "Registry tool key (claude, codex, devserver, ...)")
	addCmd.Flags().StringVarP(&addDir, "dir", "d", "", "Working directory (default: current git repo root, else cwd)")
	addCmd.Flags().StringArrayVarP(&addArgs, "arg", "a", nil, "Named template value k=v, fills {k} (repeatable)")
	addCmd.Flags().StringArrayVarP(&addEnv, "env", "e", nil, "Env var K=V to inject (repeatable)")
	addCmd.Flags().BoolVar(&addNoLaunch, "no-launch", false, "Register without starting it now")
	addCmd.MarkFlagRequired("tool")
	rmCmd.Flags().BoolVar(&rmKill, "kill", true, "Also kill its tmux window")
}

func expandUser(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

// resolveDir turns the --dir flag into an absolute working directory. When not
// given, it defaults to the current git repo root, falling back to the cwd.
func resolveDir(given string) string {
	if given != "" {
		abs, _ := filepath.Abs(expandUser(given))
		return abs
	}
	if root, ok := gitRepoRoot(); ok {
		return root
	}
	cwd, _ := os.Getwd()
	return cwd
}
