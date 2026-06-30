// Package state persists the flat registry of jobs respawn manages.
//
// A "job" is any long-running, interactive thing you want kept alive and
// resumed: an AI coding session, a dev server, a training run, a bot. One JSON
// file, one row per job. This is the cross-repo "all my long-running sessions in
// one place" record that survives reboots and drives `restore`.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Job is one managed long-running process.
type Job struct {
	Name  string            `json:"name"`
	Tool  string            `json:"tool"`
	Dir   string            `json:"dir"`
	Flags string            `json:"flags"`
	Env   map[string]string `json:"env"`
	// Args fill named {placeholders} in the tool's templates, e.g. {port},
	// {lr} — a long-running system can take many parameters.
	Args      map[string]string `json:"args"`
	SessionID string            `json:"session_id"`
}

func dir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	d := filepath.Join(base, "respawn")
	os.MkdirAll(d, 0o755)
	return d
}

func jobsPath() string { return filepath.Join(dir(), "jobs.json") }

// Load reads all jobs (empty map if none yet).
func Load() (map[string]Job, error) {
	data, err := os.ReadFile(jobsPath())
	if os.IsNotExist(err) {
		return map[string]Job{}, nil
	}
	if err != nil {
		return nil, err
	}
	jobs := map[string]Job{}
	if len(data) == 0 {
		return jobs, nil
	}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// Save writes all jobs atomically.
func Save(jobs map[string]Job) error {
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	tmp := jobsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, jobsPath())
}
