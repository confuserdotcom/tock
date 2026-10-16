package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSyncCommandAndDoctor(t *testing.T) {
	dir := t.TempDir()
	te, _ := doctorEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "sync"); code != 1 || !strings.Contains(te.errb.String(), "sync is off") {
		t.Fatalf("sync off: %d %s %s", code, out, te.errb.String())
	}
	writeFile(t, te.paths().Config, "[sync]\ndir = \""+filepath.Join(dir, "missing")+"\"\nmachine = \"mac\"\n")
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "!!  sync: no folder") {
		t.Fatalf("missing folder:\n%s", out)
	}
	writeFile(t, te.paths().Config, "[sync]\ndir = \""+dir+"\"\nmachine = \"mac\"\n")
	// ryuk's day file, as its trackd writes it.
	os.MkdirAll(filepath.Join(dir, "ryuk"), 0o700)
	line := `{"s":` + strconv.FormatInt(at(t, "11:45"), 10) + `,"e":` + strconv.FormatInt(at(t, "12:15"), 10) + `,"b":"kitty","a":"kitty","c":"Code"}` + "\n"
	os.WriteFile(filepath.Join(dir, "ryuk", "2026-10-10.jsonl"), []byte(line), 0o600)
	os.WriteFile(filepath.Join(dir, "ryuk", "machine.json"), []byte(`{"input":true}`), 0o600)

	code, out := te.run(t, "", "sync", "--days", "3")
	if code != 0 || !strings.Contains(out, "mac: wrote 3 day files, read 1") || !strings.Contains(out, "ryuk: last active Sat 10 Oct 12:15 (1 day)") {
		t.Fatalf("sync: %d\n%s%s", code, out, te.errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "mac", "2026-10-08.jsonl")); err != nil {
		t.Fatal("--days 3 writes three days")
	}
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "ok  sync: as mac with ryuk (last Sat 12:15)") {
		t.Fatalf("doctor:\n%s", out)
	}
	if _, out := te.run(t, "", "today"); !strings.Contains(out, "3h24 tracked") {
		t.Fatalf("today shows the other machine:\n%s", out)
	}
	if code, _ := te.run(t, "", "sync", "--days", "0"); code != 2 {
		t.Fatal("--days 0 accepted")
	}
}

func TestDoctorNamesMachinesWithoutInput(t *testing.T) {
	dir := t.TempDir()
	te, _ := doctorEnv(t, testRules)
	te.seed(t, daySpans()...)
	writeFile(t, te.paths().Config, "[sync]\ndir = \""+dir+"\"\nmachine = \"mac\"\n")
	os.MkdirAll(filepath.Join(dir, "ryuk"), 0o700)
	line := `{"s":` + strconv.FormatInt(at(t, "11:45"), 10) + `,"e":` + strconv.FormatInt(at(t, "12:15"), 10) + `,"b":"zathura","a":"zathura"}` + "\n"
	os.WriteFile(filepath.Join(dir, "ryuk", "2026-10-10.jsonl"), []byte(line), 0o600)
	if code, out := te.run(t, "", "sync"); code != 0 {
		t.Fatalf("sync: %d %s", code, out)
	}
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "!!  afk: ryuk does not report input; its windows over 20m are cut") {
		t.Fatalf("doctor:\n%s", out)
	}
	if _, out := te.run(t, "", "today"); !strings.Contains(out, "2h59 tracked") {
		t.Fatalf("ryuk's 30 min should be cut to 5:\n%s", out)
	}
}
