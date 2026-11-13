package main

import (
	"bufio"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/snapshot"
	"tock/internal/store"
)

// stubService records stop/start and restores the real ones after the test.
func stubService(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	oldStop, oldStart := restoreStop, restoreStart
	restoreStop = func(Env) error { calls = append(calls, "stop"); return nil }
	restoreStart = func(Env) error { calls = append(calls, "start"); return nil }
	t.Cleanup(func() { restoreStop, restoreStart = oldStop, oldStart })
	return &calls
}

// seededSnapshot opens the test DB, takes one daily snapshot of it, then
// adds a span the snapshot doesn't have.
func seededSnapshot(t *testing.T, te *tenv) snapshot.Info {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	info, err := snapshot.Write(snapshot.Opts{Dir: store.AutoDir(te.dbPath()),
		Name: snapshot.DailyName(te.now.UnixMilli(), store.LatestVersion()), Vacuum: st.BackupTo})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestRestoreList(t *testing.T) {
	te := newTestEnv(t, "")
	stubService(t)
	seededSnapshot(t, te)
	code, out := te.run(t, "", "restore")
	if code != 0 || !strings.Contains(out, "1  ") || !strings.Contains(out, "daily") {
		t.Fatalf("code %d out:\n%s\nerr: %s", code, out, te.errb)
	}
}

func TestRestoreNeedsYesWhenPiped(t *testing.T) {
	te := newTestEnv(t, "")
	calls := stubService(t)
	seededSnapshot(t, te)
	code, _ := te.run(t, "", "restore", "1")
	if code == 0 || !strings.Contains(te.errb.String(), "--yes") || len(*calls) != 0 {
		t.Fatalf("code %d calls %v err %s", code, *calls, te.errb)
	}
}

func TestRestoreSwapsAndKeepsOld(t *testing.T) {
	te := newTestEnv(t, "")
	calls := stubService(t)
	seededSnapshot(t, te)
	code, out := te.run(t, "", "restore", "1", "--yes")
	if code != 0 {
		t.Fatalf("code %d out %s err %s", code, out, te.errb)
	}
	if strings.Join(*calls, ",") != "stop,start" {
		t.Fatalf("service calls = %v", *calls)
	}
	old, _ := filepath.Glob(te.dbPath() + ".before-restore-*")
	if len(old) == 0 {
		t.Fatal("old database not kept")
	}
	if _, err := os.Stat(filepath.Join(te.DataDir, ".restore-tmp.db")); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatalf("restored db does not open: %v", err)
	}
	_ = st.Close()
}

func TestRestoreRefusesNewerSchema(t *testing.T) {
	te := newTestEnv(t, "")
	calls := stubService(t)
	dir := store.AutoDir(te.dbPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := snapshot.DailyName(te.now.UnixMilli(), store.LatestVersion()+1)
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _ := te.run(t, "", "restore", "1", "--yes")
	if code == 0 || !strings.Contains(te.errb.String(), "newer") || len(*calls) != 0 {
		t.Fatalf("code %d calls %v err %s", code, *calls, te.errb)
	}
}

// TestRestoreLeavesServicesOfOtherDataDirs: restoring into a data dir that
// isn't the one trackd and the menu bar use must not stop or start them.
func TestRestoreLeavesServicesOfOtherDataDirs(t *testing.T) {
	te := newTestEnv(t, "")
	var ran []string
	old := setupRun
	setupRun = func(name string, args ...string) (string, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return "", nil
	}
	t.Cleanup(func() { setupRun = old })
	if err := stopServices(te.Env); err != nil {
		t.Fatal(err)
	}
	if err := startServices(te.Env); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 0 {
		t.Fatalf("ran %v for a data dir trackd doesn't use", ran)
	}
}

// TestRestoreRefusesWhileHeld: another process with the store open
// makes restore refuse, name that pid, and leave tock.db as it was.
func TestRestoreRefusesWhileHeld(t *testing.T) {
	te := newTestEnv(t, "")
	calls := stubService(t)
	seededSnapshot(t, te)

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "TOCK_TEST_HOLD_DB="+te.dbPath())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Wait() })
	line, _ := bufio.NewReader(stdout).ReadString('\n')
	if strings.TrimSpace(line) != "ready" {
		t.Fatalf("helper: %q", line)
	}

	waits := 0
	te.Sleep = func(time.Duration) { waits++ }
	before := fileHash(t, te.dbPath())
	code, _ := te.run(t, "", "restore", "1", "--yes")
	if waits == 0 {
		t.Fatal("refused without waiting for a holder that may be exiting")
	}
	if code == 0 {
		t.Fatal("restore succeeded while another process held the store")
	}
	if pid := strconv.Itoa(cmd.Process.Pid); !strings.Contains(te.errb.String(), "pid "+pid) {
		t.Fatalf("error does not name pid %s: %s", pid, te.errb)
	}
	if fileHash(t, te.dbPath()) != before {
		t.Fatal("tock.db changed")
	}
	if old, _ := filepath.Glob(te.dbPath() + ".before-restore-*"); len(old) != 0 {
		t.Fatalf("kept a copy though nothing was restored: %v", old)
	}
	// Services stopped for the attempt come back.
	if strings.Join(*calls, ",") != "stop,start" {
		t.Fatalf("service calls = %v", *calls)
	}
}

func fileHash(t *testing.T, p string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// issueTestInvoice stores one invoice in te's database and writes its
// files, as tock invoice issue will.
func issueTestInvoice(t *testing.T, te *tenv) store.Invoice {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	inv, err := st.IssueInvoice(store.IssueSpec{
		Draft: store.Invoice{IssuedMS: te.now.UnixMilli(), Project: "tock", TZ: "UTC", MinorUnits: 2,
			RateMinor: 6000, RoundStepS: 1, RoundMode: "nearest", SecondsBilled: 3600, SubtotalMinor: 6000, TotalMinor: 6000},
		Loc:    time.UTC,
		Render: func(store.Invoice) ([]byte, error) { return []byte("%PDF test"), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteInvoiceFiles(store.InvoiceDir(te.dbPath()), inv); err != nil {
		t.Fatal(err)
	}
	return inv
}

// A snapshot older than an invoice would drop it from the database: the
// restore names it and needs --yes even when someone could answer y.
func TestRestoreWarnsAboutNewerInvoices(t *testing.T) {
	te := newTestEnv(t, "")
	calls := stubService(t)
	seededSnapshot(t, te)
	inv := issueTestInvoice(t, te)

	te.Interactive = true
	code, out := te.run(t, "y\n", "restore", "1")
	if code == 0 || len(*calls) != 0 || !strings.Contains(out+te.errb.String(), inv.Number) || !strings.Contains(te.errb.String(), "--yes") {
		t.Fatalf("code %d calls %v out %q err %q", code, *calls, out, te.errb)
	}
	code, out = te.run(t, "", "restore", "1", "--yes")
	if code != 0 || !strings.Contains(out, inv.Number) {
		t.Fatalf("code %d out %q err %q", code, out, te.errb)
	}
	// The files stay, so the invoice can be recovered.
	if _, err := os.Stat(filepath.Join(store.InvoiceDir(te.dbPath()), inv.Number+".pdf")); err != nil {
		t.Fatal(err)
	}
}

// With no invoice lost, restore asks as before.
func TestRestoreKeepsInvoicesInSnapshot(t *testing.T) {
	te := newTestEnv(t, "")
	calls := stubService(t)
	issueTestInvoice(t, te)
	seededSnapshot(t, te)
	te.Interactive = true
	code, out := te.run(t, "y\n", "restore", "1")
	if code != 0 || strings.Join(*calls, ",") != "stop,start" || strings.Contains(out, "invoice") {
		t.Fatalf("code %d calls %v out %q err %q", code, *calls, out, te.errb)
	}
}
