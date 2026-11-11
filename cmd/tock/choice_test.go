package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/config"
)

func (te *tenv) writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(te.ConfigDir, config.File)
	if err := os.MkdirAll(te.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestChoiceList(t *testing.T) {
	te := newTestEnv(t, "")
	te.writeConfig(t, "[ocr]\nenabled = true\n")
	code, out := te.run(t, "", "choice")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	for _, want := range []string{"on   ocr             Screen text", "off    break_screen  Break screen", "off  claude          Claude",
		"on   focus           Focus"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list lacks %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "\n"); n != len(config.Features) {
		t.Fatalf("%d lines, want %d:\n%s", n, len(config.Features), out)
	}
}

func TestChoiceSet(t *testing.T) {
	te := newTestEnv(t, "")
	p := te.writeConfig(t, "# mine\n[focus]\nnotify = true  # all of them\n")
	code, out := te.run(t, "", "choice", "break_screen", "on")
	if code != 0 || out != "Break screen: on\n" {
		t.Fatalf("exit %d out %q err %s", code, out, te.errb)
	}
	if got := readFile(t, p); got != "# mine\n[focus]\nnotify = true  # all of them\nbreak_screen = true\n" {
		t.Fatalf("config.toml = %q", got)
	}
	if got := readFile(t, p+".bak"); got != "# mine\n[focus]\nnotify = true  # all of them\n" {
		t.Fatalf("config.toml.bak = %q", got)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", fi.Mode(), err)
	}
	code, out = te.run(t, "", "choice", "notify", "off")
	if code != 0 || !strings.Contains(readFile(t, p), "notify = false  # all of them") {
		t.Fatalf("exit %d out %q:\n%s", code, out, readFile(t, p))
	}
}

// G4, AC2: a group is one key; its members keep theirs and wait.
func TestChoiceGroup(t *testing.T) {
	te := newTestEnv(t, "")
	src := "# mine\n[focus]\nwake_screen = true\n"
	p := te.writeConfig(t, src)
	if code, out := te.run(t, "", "choice", "focus", "off"); code != 0 || out != "Focus: off\n" {
		t.Fatalf("exit %d out %q err %s", code, out, te.errb)
	}
	if got := readFile(t, p); got != src+"enabled = false\n" {
		t.Fatalf("config.toml = %q", got)
	}
	_, out := te.run(t, "", "choice")
	if !strings.Contains(out, "on     wake_screen   Wake screen  (waits for Focus)") || !strings.Contains(out, "off  focus           Focus\n") {
		t.Fatalf("list:\n%s", out)
	}
	if code, _ := te.run(t, "", "choice", "focus", "on"); code != 0 || readFile(t, p) != src {
		t.Fatalf("on again: exit %d, %q", code, readFile(t, p))
	}
}

// No config.toml yet: the file is made with the one setting.
func TestChoiceSetNoFile(t *testing.T) {
	te := newTestEnv(t, "")
	if code, _ := te.run(t, "", "choice", "slack", "on"); code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	p := filepath.Join(te.ConfigDir, config.File)
	if got := readFile(t, p); got != "[slack]\nenabled = true\n" {
		t.Fatalf("config.toml = %q", got)
	}
	if !strings.Contains(te.out.String(), "tock slack login") {
		t.Fatalf("no login hint: %q", te.out)
	}
	if _, err := os.Stat(p + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("backup of a file that did not exist")
	}
}

func TestChoiceSetErrors(t *testing.T) {
	te := newTestEnv(t, "")
	for _, args := range [][]string{{"choice", "nope", "on"}, {"choice", "ocr", "maybe"}, {"choice", "ocr"}} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Fatalf("%v: exit %d", args, code)
		}
	}
	// A config that does not load is left alone.
	broken := "[focus]\nnotify = 3\n"
	p := te.writeConfig(t, broken)
	if code, _ := te.run(t, "", "choice", "ocr", "on"); code != 1 || readFile(t, p) != broken {
		t.Fatalf("exit %d, file %q", code, readFile(t, p))
	}
	// A setting tock cannot place is left alone too.
	inline := "ocr = { enabled = false }\n"
	te.writeConfig(t, inline)
	if code, _ := te.run(t, "", "choice", "ocr", "on"); code != 1 || readFile(t, p) != inline || !strings.Contains(te.errb.String(), "by hand") {
		t.Fatalf("exit %d, file %q, err %q", code, readFile(t, p), te.errb)
	}
}

// Turning screen text on asks trackd for the Screen Recording grant on a
// Mac, and only there.
func TestChoiceOCRAsksGrant(t *testing.T) {
	te := newTestEnv(t, "")
	te.run(t, "", "choice", "ocr", "on")
	if !strings.Contains(te.out.String(), "Screen Recording") {
		t.Fatalf("no grant step: %q", te.out)
	}
	te.Linux = true
	te.run(t, "", "choice", "ocr", "off")
	te.run(t, "", "choice", "ocr", "on")
	if strings.Contains(te.out.String(), "Screen Recording") {
		t.Fatalf("grant asked on Linux: %q", te.out)
	}
}

func TestChoiceAlwaysOpen(t *testing.T) {
	te := newTestEnv(t, "")
	if choiceAlways(te.Env) {
		t.Fatal("always before Remember")
	}
	if err := rememberAlways(te.Env); err != nil {
		t.Fatal(err)
	}
	if !choiceAlways(te.Env) {
		t.Fatal("not remembered")
	}
}

func TestOpenDashboard(t *testing.T) {
	te := newTestEnv(t, "")
	var opened string
	te.Open = func(u string) error { opened = u; return nil }
	te.StartDashboard = func() (string, error) { return "http://127.0.0.1:9/#t", nil }
	link, err := openDashboard(te.Env)
	if err != nil || link != "http://127.0.0.1:9/#t" || opened != link {
		t.Fatalf("link %q opened %q err %v", link, opened, err)
	}
	te.StartDashboard = func() (string, error) { return "", errors.New("dash: no link") }
	if _, err := openDashboard(te.Env); err == nil {
		t.Fatal("no error")
	}
}

// The real starter reads the link tock web writes to its log.
func TestDashLink(t *testing.T) {
	te := newTestEnv(t, "")
	log := filepath.Join(t.TempDir(), "web.log")
	calls := 0
	te.Sleep = func(d time.Duration) {
		calls++
		if calls == 2 {
			os.WriteFile(log, []byte("tock web: http://127.0.0.1:5/#k\nOnly this machine can connect.\n"), 0o600)
		}
	}
	link, err := waitLink(te.Env, log)
	if err != nil || link != "http://127.0.0.1:5/#k" {
		t.Fatalf("link %q err %v", link, err)
	}
	te.Sleep = func(time.Duration) {}
	if _, err := waitLink(te.Env, filepath.Join(t.TempDir(), "none.log")); err == nil {
		t.Fatal("no error without a link")
	}
}

// detach runs a command that outlives the call, its output in the file.
func TestDetach(t *testing.T) {
	te := newTestEnv(t, "")
	log := filepath.Join(t.TempDir(), "web.log")
	out, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := detach("/bin/sh", []string{"-c", "echo 'tock web: http://127.0.0.1:1/#x'"}, out); err != nil {
		t.Fatal(err)
	}
	te.Sleep = time.Sleep
	if link, err := waitLink(te.Env, log); err != nil || link != "http://127.0.0.1:1/#x" {
		t.Fatalf("link %q err %v", link, err)
	}
}
