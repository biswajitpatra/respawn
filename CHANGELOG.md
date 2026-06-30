# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Initial release: a config-driven registry of `start`/`resume` command
  templates for long-running, interactive tmux jobs.
- `add`, `ls`, `snapshot`, `restore`, `restart`, `stop`, `rm`, `attach`,
  `tools` commands.
- `stop NAME` kills a running job's window but keeps it registered (snapshots
  the session id first so a later `restart`/`restore` resumes it).
- `add` now defaults `--dir` to the current git repo root (falling back to the
  cwd) when not given.
- Session-id capture strategies: `newest_file`, `arg`, `none`.
- Built-in tool entries for `claude`, `codex`, `gemini`, `opencode`.
- Named template values via `-a key=val` and a verbatim flag tail after `--`.
- macOS reboot persistence via launchd (`install-boot` / `uninstall-boot`).
