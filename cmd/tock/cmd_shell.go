package main

import (
	"fmt"
	"strings"
)

// zshHook appends "<unix ms>\t<$PWD>" to the shell log at every prompt and
// command, with builtins only so a prompt never forks. %s is the quoted log
// path.
const zshHook = `# tock: record the working directory at each prompt and command.
zmodload -F zsh/datetime p:EPOCHREALTIME
typeset -g _tock_shell_log=%s
[[ -e $_tock_shell_log ]] || ( umask 077; mkdir -p -- "${_tock_shell_log:h}" && : >> "$_tock_shell_log" )
_tock_dir() {
  [[ $PWD == *[$'\t\n']* ]] && return
  printf '%%.0f\t%%s\n' $(( EPOCHREALTIME * 1000 )) "$PWD" >> "$_tock_shell_log"
}
autoload -Uz add-zsh-hook
add-zsh-hook precmd _tock_dir
add-zsh-hook preexec _tock_dir
`

// cmdShellInit prints the shell hook, for eval "$(tock shell-init zsh)".
func cmdShellInit(env Env, args []string) error {
	if len(args) != 1 || args[0] != "zsh" {
		return usagef("usage: tock shell-init zsh")
	}
	fmt.Fprintf(env.Stdout, zshHook, shQuote(env.shellLogPath()))
	return nil
}

// shQuote single-quotes s for sh and zsh.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
