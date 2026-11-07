package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if _, err := store.Open(te.dbPath()); err != nil {
		t.Fatalf("restored db does not open: %v", err)
	}
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
	if err := stopForRestore(te.Env); err != nil {
		t.Fatal(err)
	}
	if err := startAfterRestore(te.Env); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 0 {
		t.Fatalf("ran %v for a data dir trackd doesn't use", ran)
	}
}
