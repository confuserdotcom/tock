# Shell hook: terminal time by working directory

tock sees a terminal's window title, not where your shell is. The shell hook
records the working directory each time zsh shows a prompt or starts a
command, so rules can map terminal time to projects in any terminal, even
while a program such as Claude Code or nvim owns the title.

## Install

Add to `~/.zshrc`:

```zsh
eval "$(tock shell-init zsh)"
```

Open a new shell. Each prompt and command appends one line to
`~/.local/state/tock/shell.log`:

```
<unix ms>	<working directory>
```

The hook uses zsh builtins only, so a prompt never starts a process. The log
is owner-only, and it grows about 60 bytes per command (around 10 MB a year).

## Map directories to projects

Rules gain a `dir` field, a regex like `title` and `url`. Directories under
your home are written `~/…`:

```toml
[[rule]]
dir = '^~/code/tock(/|$)'
project = "tock"
```

A `dir` rule only matches terminal views that have a directory. The category
(Code) comes from your terminal rule, so project rules only need `project`.
`tock rules stats` shows how much time each rule decided.

## Which apps are terminals

These terminals get directories by default: WezTerm, Terminal, iTerm2,
Ghostty, kitty, Alacritty and Warp. To change the list, put the bundle ids in
`~/.config/tock/config.toml`:

```toml
[shell]
terminals = ["com.github.wez.wezterm", "com.mitchellh.ghostty"]
```

## How a view gets its directory

A terminal view gets the directory that was current for most of its time.
"Current" means from the last prompt or command of any shell, so with several
tabs open, the directory belongs to the shell you used last. A long-running
program keeps the directory of the command that started it. Switching
between two tabs that each run a long program without returning to a prompt
keeps the directory of whichever one started last.

## Privacy

Directories stay on this Mac. They are never sent to Claude and are not part
of `tock export`. `tock backup` includes `shell.log`. To stop recording,
remove the line from `~/.zshrc`. To forget everything, delete the log.
