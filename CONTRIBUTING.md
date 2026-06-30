# Contributing

Thanks for your interest in improving **respawn**.

## Development

```bash
go build -o respawn .     # build
go test ./...             # run tests
go vet ./...              # static checks
make check                # vet + test (what CI runs)
```

CI runs `gofmt`, `go vet`, and `go test` on every push and PR — keep them green.

## Layout

- `main.go` — entrypoint.
- `cmd/` — the cobra CLI: one command per concern, shared rendering in `root.go`.
- `internal/config/` — the tool registry (`tools_default.toml` is embedded);
  loads defaults and merges the user's `~/.config/respawn/tools.toml` fieldwise.
- `internal/state/` — the `jobs.json` store (the flat cross-repo job registry).
- `internal/capture/` — session-id capture (`newest_file`, `arg`, `none`).
- `internal/proc/` — process-tree walk to find a tool under a tmux pane.
- `internal/tmux/` — the tmux command interface (shells out to `tmux`).
- `internal/boot/` — launchd integration for reboot persistence (macOS).

The dependency rule is one-directional: `cmd` → `internal/*`; the `internal`
packages don't import `cmd`.

## Adding a tool

No code needed — add an entry to `internal/config/tools_default.toml` (or, for
your own machine, `~/.config/respawn/tools.toml`):

1. `detect` — the binary name to find in the process tree.
2. `capture` — how to recover its session id (`newest_file` / `arg` / `none`).
3. `start` / `resume` — command templates using `{name} {dir} {session_id}
   {flags}` and any named `{placeholders}` (filled via `-a key=val`).

If a tool needs a genuinely new capture *mechanism*, add a `kind` in
`internal/capture/capture.go` and a test in `capture_test.go`.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/):
`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`. Keep the subject
imperative and under ~72 chars.

## Pull requests

1. Fork and branch from `main`.
2. Add or update a test for any behavior change.
3. Keep the diff focused; one concern per PR.
4. Run `make check` before pushing.
5. Describe the change and how you verified it.

## Reporting bugs / ideas

Open an issue using the templates. Please include your OS, `tmux -V`, and the
exact `respawn` command and output.
