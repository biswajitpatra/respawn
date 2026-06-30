# respawn

**Persist and resurrect long-running tmux sessions — and bring each one back
*where it left off*, not from scratch.**

`tmux` keeps your sessions alive when SSH drops or your laptop sleeps. It does
**not** survive a reboot — and even the tools that restore your *layout*
relaunch every program **fresh**: your AI coding agent forgets the conversation,
your training run restarts from epoch 0, your dev server is just… restarted.

`respawn` closes that gap with one small idea: **a per-tool registry of `start`
and `resume` command templates.** It captures each session's resumable id (a
Claude Code session id, an OpenCode `-s` id, a checkpoint path — whatever the
tool exposes) and, on reboot, recreates the tmux windows and **replays the
*resume* command** so the thing comes back with its state intact.

It's a single static Go binary, tool-agnostic, and config-driven. AI coding CLIs
are the motivating case, but **anything long-running and interactive** fits — a
dev server, a training run, a bot, a tunnel.

> Status: early (`v0.1`). macOS-first (launchd boot integration). Linux works
> for everything except the boot hook — use a systemd user service (below).

---

## Why this exists (and how it differs)

| Tool | Survives disconnect | Survives **reboot** | Comes back **resumed** | Any tool, config-driven |
|---|:--:|:--:|:--:|:--:|
| `tmux` / `screen` | ✅ | ❌ | ❌ | — |
| [`abduco`](https://github.com/martanne/abduco) / `dtach` / [`zmx`](https://github.com/neurosnap/zmx) | ✅ | ❌ | ❌ | — |
| [Zellij resurrection](https://zellij.dev/documentation/session-resurrection.html) | ✅ | partial | ❌ (re-runs fresh) | ❌ |
| [`tmux-resurrect`](https://github.com/tmux-plugins/tmux-resurrect) + `continuum` | ✅ | ✅ (layout) | ❌ (fresh; conservative list) | ❌ |
| [`pm2`](https://pm2.keymetrics.io/) / [Supervisor](http://supervisord.org/) / systemd | n/a (headless) | ✅ | ❌ (fresh restart) | partial |
| [`tmux-assistant-resurrect`](https://github.com/timvw/tmux-assistant-resurrect) | ✅ | ✅ | ✅ (resume id) | ❌ (4 CLIs, hardcoded) |
| **`respawn`** | ✅ | ✅ | ✅ | ✅ |

The closest prior art is **`tmux-assistant-resurrect`** — it proves the
resume-the-session-id mechanism works for Claude/Codex/OpenCode/Pi, riding
`tmux-resurrect`/`continuum`. Its own docs note it is *"NOT templatized for
user-defined assistants"* — adding a tool means editing shell scripts.
`respawn`'s one job is to make that part **config, not code**, generalize it to
*any* long-running command, and give it a flat cross-repo management surface.

*(Comparison rows are documented from each linked project; the "resumed" column
reflects each tool's stated restore behavior.)*

## Use cases

`respawn` is for anything you want **always coming back, attachable, and
resumed**:

- **AI coding agents** — `claude` / `codex` / `gemini` / `opencode` resume the
  conversation, not a blank session. *(motivating case)*
- **ML training / fine-tuning over SSH** — `resume = "python train.py
  --resume-from-checkpoint {session_id}"`, where the captured id is your latest
  checkpoint file.
- **Dev servers & watchers** — `vite`, `webpack`, `next dev`. No resume id;
  `respawn` just relaunches them at login.
- **Bots / game servers / tunnels** — Discord bots, `cloudflared`, `ngrok`:
  restart-on-reboot in an attachable pane.
- **Data pipelines / REPLs / 24-7 agent loops** — re-enter stateful work after
  a reboot, in a pane you can attach to and inspect.

The unit is always: *a named, attachable tmux window + a `start`/`resume`
template + a captured session id.*

## Install

Requires `tmux`. A single static binary, no runtime.

```bash
# from source (Go ≥ 1.22)
go install github.com/biswajitpatra/respawn@latest

# or clone + build
git clone https://github.com/biswajitpatra/respawn && cd respawn
make build && cp ./respawn /usr/local/bin/
```

## Quickstart

```bash
# register + launch a job (window name = "frontend")
respawn add frontend -t claude -d ~/work/app

# a job with named params and a verbatim flag tail after `--`
respawn add trainer -t trainer -d ~/ml/run -a lr=0.01 -- --mixed-precision

respawn ls            # every job across every repo, with live status
respawn attach        # jump into the tmux session (focus one: `attach frontend`)

# make it survive reboots (macOS): restore at login + snapshot every 5 min
respawn install-boot
```

After a reboot your windows are recreated and each job is relaunched with its
**resume** command (Claude with `--resume <id>`, a trainer from its checkpoint,
a dev server fresh).

```
$ respawn ls
NAME      TOOL    STATUS   SESSION       DIR
frontend  claude  running  a1b2c3d4e5f6  /Users/me/work/app
trainer   python  idle     ckpt-00420    /Users/me/ml/run
bot       node    down     -             /Users/me/bots/discord
```

### args vs flags

- **`-a key=value`** fills a **named** `{key}` placeholder the template declares
  (`{port}`, `{lr}`). Structured and self-documenting.
- **everything after `--`** becomes the verbatim `{flags}` tail — the escape
  hatch for arbitrary options you don't want to templatize.

## How it works

1. **Registry** (`internal/config/tools_default.toml`, overridden at
   `~/.config/respawn/tools.toml`) — each tool defines `detect`, a `capture`
   rule, and `start` / `resume` templates with `{name} {dir} {session_id}
   {flags}` plus any named `-a` values. Declared `env` vars are captured and
   re-injected.
2. **State** (`~/.local/state/respawn/jobs.json`) — the flat list of jobs
   `(name, tool, dir, flags, env, args, session_id)`. The cross-repo record.
3. **Capture** — `snapshot` walks each window's process tree to find the tool
   and reads its session id (from a transcript file, the command line, or
   nothing). Run on a timer so the last-known id is always fresh on disk.
4. **Restore** — recreates each window in one tmux session and replays the
   resume command.

### Adding your own tool — config, not code

```toml
# ~/.config/respawn/tools.toml
[tools.trainer]
detect  = "python"
start   = "python train.py --lr {lr} {flags}"
resume  = "python train.py --lr {lr} --resume-from-checkpoint {session_id} {flags}"
capture = { kind = "newest_file", base = "./checkpoints", project = "none", glob = "*.ckpt" }
env     = ["CUDA_VISIBLE_DEVICES"]
```

Capture kinds: `newest_file` (newest matching file's stem is the id — survives
process exit), `arg` (regex over the command line), `none` (resume == start).

### Linux boot persistence

`install-boot` is macOS/launchd only. On Linux, run `respawn restore` from a
systemd **user** service with lingering enabled:

```ini
# ~/.config/systemd/user/respawn.service
[Service]
Type=oneshot
ExecStart=%h/go/bin/respawn restore
[Install]
WantedBy=default.target
```
```bash
loginctl enable-linger "$USER"
systemctl --user enable respawn.service
# add a respawn.timer calling `respawn snapshot` every few minutes
```

## Caveats

- **State, not action.** Like the underlying CLIs' own resume, this restores the
  *context*, not an in-flight tool call. The job remembers; you re-prompt it to
  continue.
- **One session per dir, by default.** `newest_file` capture picks the most
  recent transcript in a directory; for two jobs of the same tool in one dir,
  use `arg` capture or distinct rules.
- **`gemini` / `codex` resume flags are best-effort defaults** — verify against
  your CLI version and override in your config.

## Development

```bash
make build   # build ./respawn
make test    # go test ./...
make check   # vet + test
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
