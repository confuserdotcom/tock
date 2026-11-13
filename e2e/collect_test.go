//go:build e2e

// Package e2e runs the real trackd against a real desktop: the CI job
// opens windows, this test switches focus between them through E2E_FOCUS,
// kills trackd with no warning, starts it again and checks what tock
// exports. Nothing here is faked: the source, the store and recovery are
// the ones users run.
//
//	E2E_TITLES  comma-separated window titles the job has opened (two or more)
//	E2E_FOCUS   a shell command that focuses one; {title} is replaced
//	E2E_DESKTOP optional TOCK_DESKTOP for trackd (hyprland, sway, x11)
//
// go test -tags e2e ./e2e -v -count=1
package e2e

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type row struct {
	Start     string `json:"start"`
	End       string `json:"end"`
	DurationS int64  `json:"duration_s"`
	App       string `json:"app"`
	Title     string `json:"title"`
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// build compiles one command into dir.
func build(t *testing.T, dir, pkg string) string {
	t.Helper()
	out := filepath.Join(dir, exe(filepath.Base(pkg)))
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = ".."
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, b)
	}
	return out
}

func focus(t *testing.T, title string) {
	t.Helper()
	tmpl := os.Getenv("E2E_FOCUS")
	line := strings.ReplaceAll(tmpl, "{title}", title)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-Command", line)
	} else {
		cmd = exec.Command("sh", "-c", line)
	}
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("focus %q: %v\n%s", title, err, b)
	}
}

type daemon struct {
	cmd *exec.Cmd
	log *bytes.Buffer
}

func start(t *testing.T, trackd string, env []string) *daemon {
	t.Helper()
	d := &daemon{log: &bytes.Buffer{}}
	d.cmd = exec.Command(trackd)
	d.cmd.Env = env
	d.cmd.Stdout = d.log
	d.cmd.Stderr = d.log
	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start trackd: %v", err)
	}
	return d
}

// kill ends trackd with no chance to flush: SIGKILL, TerminateProcess.
func (d *daemon) kill(t *testing.T) {
	t.Helper()
	if err := d.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill trackd: %v", err)
	}
	_ = d.cmd.Wait()
}

var recovered = regexp.MustCompile(`recovered open spans.*count=(\d+)`)

// recoveredCount reads the count from trackd's log, whichever file the
// platform writes it to.
func recoveredCount(t *testing.T, d *daemon, stateDir string) int {
	t.Helper()
	text := d.log.String()
	if b, err := os.ReadFile(filepath.Join(stateDir, "trackd.log")); err == nil {
		text += string(b)
	}
	m := recovered.FindAllStringSubmatch(text, -1)
	if m == nil {
		t.Fatalf("no recovery line in trackd's log:\n%s", text)
	}
	n, _ := strconv.Atoi(m[len(m)-1][1])
	return n
}

// switchFor focuses each title in turn for hold, rounds times.
func switchFor(t *testing.T, titles []string, hold time.Duration, rounds int) {
	for i := 0; i < rounds; i++ {
		for _, w := range titles {
			focus(t, w)
			time.Sleep(hold)
		}
	}
}

func TestCollectKillRecover(t *testing.T) {
	titles := strings.Split(os.Getenv("E2E_TITLES"), ",")
	if os.Getenv("E2E_FOCUS") == "" || len(titles) < 2 {
		t.Skip("set E2E_TITLES (two or more) and E2E_FOCUS; see the package comment")
	}
	bin := t.TempDir()
	trackd := build(t, bin, "./cmd/trackd")
	tock := build(t, bin, "./cmd/tock")

	root := t.TempDir()
	dirs := map[string]string{
		"TOCK_DATA_DIR":   filepath.Join(root, "data"),
		"TOCK_CONFIG_DIR": filepath.Join(root, "config"),
		"TOCK_STATE_DIR":  filepath.Join(root, "state"),
	}
	env := os.Environ()
	for k, v := range dirs {
		if err := os.MkdirAll(v, 0o700); err != nil {
			t.Fatal(err)
		}
		env = append(env, k+"="+v)
	}
	if d := os.Getenv("E2E_DESKTOP"); d != "" {
		env = append(env, "TOCK_DESKTOP="+d)
	}

	// Run 1: record, then die mid-span.
	d1 := start(t, trackd, env)
	time.Sleep(3 * time.Second) // open the store, first poll
	switchFor(t, titles, 5*time.Second, 2)
	focus(t, titles[0])
	time.Sleep(6 * time.Second) // past a heartbeat, so the open span is on disk
	d1.kill(t)
	t.Logf("run 1 log:\n%s", d1.log.String())

	// Run 2: the open span from run 1 is recovered, recording goes on.
	d2 := start(t, trackd, env)
	time.Sleep(3 * time.Second)
	if n := recoveredCount(t, d2, dirs["TOCK_STATE_DIR"]); n < 1 {
		t.Errorf("run 2 recovered %d open spans, want at least 1", n)
	}
	switchFor(t, titles, 5*time.Second, 1)
	time.Sleep(3 * time.Second)
	d2.kill(t)
	t.Logf("run 2 log:\n%s", d2.log.String())

	// What a user sees.
	today := time.Now().Format("2006-01-02")
	cmd := exec.Command(tock, "export", "--format", "json", "--from", today, "--to", today)
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("tock export: %v\n%s", err, errb.String())
	}
	var rows []row
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("export json: %v\n%s", err, out.String())
	}
	var total int64
	seen := map[string]int64{}
	for _, r := range rows {
		total += r.DurationS
		for _, w := range titles {
			if strings.Contains(r.Title, w) || strings.Contains(r.App, w) {
				seen[w] += r.DurationS
			}
		}
	}
	for _, w := range titles {
		if seen[w] == 0 {
			t.Errorf("no recorded time on %q", w)
		}
	}
	// Recorded: about 20 s + 6 s in run 1, 10 s + 3 s in run 2. Allow the
	// first poll and the gaps around each start.
	if total < 25 || total > 60 {
		t.Errorf("recorded %d s in all, want 25..60", total)
	}
	logRows(t, rows)

	// The database is whole after two kills.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dirs["TOCK_DATA_DIR"], "tock.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ok string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&ok); err != nil || ok != "ok" {
		t.Errorf("integrity_check = %q, %v", ok, err)
	}
}

func logRows(t *testing.T, rows []row) {
	var b strings.Builder
	w := bufio.NewWriter(&b)
	for _, r := range rows {
		w.WriteString(r.Start + "  " + strconv.FormatInt(r.DurationS, 10) + "s  " + r.App + "  " + r.Title + "\n")
	}
	w.Flush()
	t.Logf("exported spans:\n%s", b.String())
}
