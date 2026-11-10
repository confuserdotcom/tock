package store

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestExclusiveRefusedWhileOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tock.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Exclusive(path)
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("Exclusive with the store open: err = %v, want HeldError", err)
	}
	if len(held.Holders) != 1 || held.Holders[0].PID != os.Getpid() {
		t.Fatalf("holders = %+v, want this process", held.Holders)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	release, err := Exclusive(path)
	if err != nil {
		t.Fatalf("Exclusive after Close: %v", err)
	}
	release()
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "holders", strconv.Itoa(os.Getpid()))); !os.IsNotExist(err) {
		t.Fatalf("holder file left after Close: %v", err)
	}
}

// Two stores in one process share one holder file; it goes with the last.
func TestHolderFileCountsOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tock.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hf := filepath.Join(filepath.Dir(path), "holders", strconv.Itoa(os.Getpid()))
	_ = a.Close()
	if _, err := os.Stat(hf); err != nil {
		t.Fatalf("holder file gone while a store is still open: %v", err)
	}
	_ = b.Close()
	if _, err := os.Stat(hf); !os.IsNotExist(err) {
		t.Fatalf("holder file left after the last Close: %v", err)
	}
}

func TestHoldersSkipsDeadPIDs(t *testing.T) {
	dir := t.TempDir()
	hd := filepath.Join(dir, "holders")
	if err := os.MkdirAll(hd, 0o700); err != nil {
		t.Fatal(err)
	}
	// Pid 1 is alive on every Unix; a huge pid is not.
	_ = os.WriteFile(filepath.Join(hd, "1"), []byte("launchd"), 0o600)
	_ = os.WriteFile(filepath.Join(hd, "99999999"), []byte("gone"), 0o600)
	got := holders(filepath.Join(dir, "tock.db"))
	if len(got) != 1 || got[0].PID != 1 || got[0].Name != "launchd" {
		t.Fatalf("holders = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(hd, "99999999")); !os.IsNotExist(err) {
		t.Fatal("stale holder file not removed")
	}
}

func TestHeldErrorMessage(t *testing.T) {
	e := &HeldError{Holders: []Holder{{4123, "tock web"}, {4188, "tock"}}}
	want := "close these first: tock web (pid 4123), tock (pid 4188)"
	if e.Error() != want {
		t.Fatalf("got %q", e.Error())
	}
	if (&HeldError{}).Error() == "" {
		t.Fatal("empty message without holders")
	}
}
