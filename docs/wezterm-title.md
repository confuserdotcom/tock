# WezTerm window title with the project directory

tock sees only the terminal's window title. By default WezTerm shows the
running program's title (for example a Claude Code task name), which says
nothing about which project you are in. This hook prefixes the title with the
basename of the active pane's working directory, so rules can map terminal
time to projects:

```
tock │ ✳ fix tests
ATLAS │ nvim scan.go
```

## Install

Add this to `~/.wezterm.lua` (or `~/.config/wezterm/wezterm.lua`), above the
final `return config`:

```lua
wezterm.on('format-window-title', function(tab, pane, tabs, panes, config)
  local p = tab.active_pane
  local cwd = p.current_working_dir
  local path = ''
  if cwd then
    -- Recent WezTerm returns a Url object; older versions a "file://" string.
    path = type(cwd) == 'userdata' and cwd.file_path or tostring(cwd)
  end
  local dir = (path:gsub('/+$', '')):match('([^/]+)$') or '~'
  return dir .. ' │ ' .. p.title
end)
```

The separator is `│` (U+2502, box drawing), surrounded by spaces. tock's
title cleaner keeps it, and the starter rules match titles like `^tock │`.

WezTerm learns the working directory from the shell (OSC 7) or, for local
panes on macOS, from the foreground process. If the prefix shows `~` while you
are inside a project, enable shell integration:
https://wezterm.org/shell-integration.html

## Map directories to projects

In `~/.config/tock/rules.toml`, one rule per project directory:

```toml
[[project]]
name = "tock"

[[rule]]
bundle = ["com.github.wez.wezterm"]
title = '^tock │'
project = "tock"
```

The category (Code) comes from the separate WezTerm rule, so project rules
only need `project`.
