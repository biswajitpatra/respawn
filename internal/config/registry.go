// Package config loads the tool registry: shipped defaults merged with the
// user's overrides. The registry is the heart of respawn — each entry teaches
// it how to recognize a tool, capture its resumable session id, and build the
// START and RESUME commands.
package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

//go:embed tools_default.toml
var defaultsTOML string

// Capture describes how to recover a tool's resumable session id.
type Capture struct {
	Kind    string `toml:"kind"`
	Base    string `toml:"base"`
	Project string `toml:"project"`
	Glob    string `toml:"glob"`
	Pattern string `toml:"pattern"`
}

// ToolSpec is one registry entry.
type ToolSpec struct {
	Detect         string   `toml:"detect"`
	Start          string   `toml:"start"`
	Resume         string   `toml:"resume"`
	ResumeFallback string   `toml:"resume_fallback"`
	Capture        Capture  `toml:"capture"`
	Env            []string `toml:"env"`
}

type file struct {
	Tools map[string]ToolSpec `toml:"tools"`
}

// UserConfigPath is where users override or add tools.
func UserConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "respawn", "tools.toml")
}

func merge(base, o ToolSpec) ToolSpec {
	if o.Detect != "" {
		base.Detect = o.Detect
	}
	if o.Start != "" {
		base.Start = o.Start
	}
	if o.Resume != "" {
		base.Resume = o.Resume
	}
	if o.ResumeFallback != "" {
		base.ResumeFallback = o.ResumeFallback
	}
	if o.Capture.Kind != "" {
		base.Capture = o.Capture
	}
	if len(o.Env) > 0 {
		base.Env = o.Env
	}
	return base
}

// Load returns the merged registry.
func Load() (map[string]ToolSpec, error) {
	var def file
	if _, err := toml.Decode(defaultsTOML, &def); err != nil {
		return nil, fmt.Errorf("decoding built-in registry: %w", err)
	}
	reg := def.Tools
	if reg == nil {
		reg = map[string]ToolSpec{}
	}

	path := UserConfigPath()
	if data, err := os.ReadFile(path); err == nil {
		var user file
		if _, err := toml.Decode(string(data), &user); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", path, err)
		}
		for name, spec := range user.Tools {
			if existing, ok := reg[name]; ok {
				reg[name] = merge(existing, spec)
			} else {
				reg[name] = spec
			}
		}
	}
	return reg, nil
}

// Get returns one tool spec or a helpful error listing known tools.
func Get(name string) (ToolSpec, error) {
	reg, err := Load()
	if err != nil {
		return ToolSpec{}, err
	}
	spec, ok := reg[name]
	if !ok {
		known := make([]string, 0, len(reg))
		for k := range reg {
			known = append(known, k)
		}
		sort.Strings(known)
		return ToolSpec{}, fmt.Errorf("unknown tool %q. Known: %v. Add it in %s",
			name, known, UserConfigPath())
	}
	return spec, nil
}
