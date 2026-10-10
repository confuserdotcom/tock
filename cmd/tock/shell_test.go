package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestShellInitZsh(t *testing.T) {
	te := newTestEnv(t, "")
	te.StateDir = filepath.Join(t.TempDir(), "it's state")
	code, out := te.run(t, "", "shell-init", "zsh")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	for _, want := range []string{
		"add-zsh-hook precmd _tock_dir",
		"add-zsh-hook preexec _tock_dir",
		`typeset -g _tock_shell_log='` + strings.ReplaceAll(te.shellLogPath(), "'", `'\''`) + "'",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if code, _ := te.run(t, "", "shell-init", "bash"); code != 2 {
		t.Fatalf("bash exit %d, want 2", code)
	}

	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("no zsh")
	}
	if b, err := exec.Command(zsh, "-n", "-c", out).CombinedOutput(); err != nil {
		t.Fatalf("zsh -n: %v %s", err, b)
	}
	cmd := exec.Command(zsh, "-f", "-c", out+"\ncd /\n_tock_dir\ncd /tmp\n_tock_dir\n")
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zsh run: %v %s", err, b)
	}
	fi, err := os.Stat(te.shellLogPath())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("log mode: %v %v", fi, err)
	}
	b, _ := os.ReadFile(te.shellLogPath())
	if !regexp.MustCompile(`^\d{13}\t/\n\d{13}\t/tmp\n$`).Match(b) {
		t.Fatalf("log = %q", b)
	}
}
