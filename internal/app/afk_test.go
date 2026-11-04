package app

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/store"
	"tock/internal/syncdir"
	"tock/internal/update"
)

const afkRules = apptest.Rules + `
[[category]]
name = "Reading"
kind = "deep"
afk = "10m"

[[rule]]
bundle = ["pdf"]
category = "Reading"
`

func TestAFKWindowKeepsShortPauseInReading(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "09:30", Bundle: "pdf", App: "PDF", Title: "book"},
		apptest.Span{ID: 2, Start: "09:30", End: "09:38", Bundle: "tock.idle", App: "Idle"},
		apptest.Span{ID: 3, Start: "09:38", End: "10:00", Bundle: "term", App: "WezTerm", Title: "x"},
		apptest.Span{ID: 4, Start: "10:00", End: "10:08", Bundle: "tock.idle", App: "Idle"},
		apptest.Span{ID: 5, Start: "10:08", End: "10:10", Bundle: "term", App: "WezTerm", Title: "x"},
	)
	a := open(t, e)
	got := timeline(t, a)
	for _, want := range []string{"09:00-09:38 PDF Reading", "10:00-10:08 Idle "} {
		if !strings.Contains(got, want) {
			t.Errorf("timeline lacks %q:\n%s", want, got)
		}
	}
}

func TestAFKPauseAcrossMidnight(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "23:00", End: "23:57", Bundle: "pdf", App: "PDF", Title: "book", Day: -1},
		apptest.Span{ID: 2, Start: "23:57", Bundle: "tock.idle", App: "Idle", Day: -1},
		apptest.Span{ID: 3, Start: "00:05", End: "00:30", Bundle: "pdf", App: "PDF", Title: "book"},
	)
	// Fix span 2's end on the next day.
	db := e.RawDB(t)
	defer db.Close()
	if _, err := db.Exec(`UPDATE span SET "end" = ? WHERE id = 2`, apptest.At(t, "00:05", 0)); err != nil {
		t.Fatal(err)
	}
	a := open(t, e)
	if got := timeline(t, a); !strings.HasPrefix(got, "00:00-00:30 PDF Reading") {
		t.Fatalf("timeline:\n%s", got)
	}
}

func TestAFKStaleWithoutInput(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "00:00", End: "06:33", Bundle: "pdf", App: "PDF", Title: "L'Étranger"})
	a := open(t, e)
	a.input = false
	a.inputSince = a.loadInputSince("linux", false)
	got := timeline(t, a)
	if !strings.Contains(got, "00:00-00:10 PDF Reading") || !strings.Contains(got, "00:10-06:33 Idle") {
		t.Fatalf("timeline:\n%s", got)
	}
}

// Once input is reported, the time before it is still judged by the stale
// rule; the time after is not.
func TestAFKStaleBeforeInputBegan(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "00:00", End: "06:33", Bundle: "pdf", App: "PDF", Title: "L'Étranger"},
		apptest.Span{ID: 2, Start: "09:00", End: "10:00", Bundle: "pdf", App: "PDF", Title: "L'Étranger"})
	a := open(t, e)
	a.input = true
	e.Clock = time.UnixMilli(apptest.At(t, "08:00", 0)) // hooked at 08:00
	a.now = e.NowFunc()
	a.inputSince = a.loadInputSince("linux", true)
	e.Clock = apptest.Now
	a.now = e.NowFunc()
	if again := a.loadInputSince("linux", true); again != a.inputSince {
		t.Fatalf("input since moved: %d, was %d", again, a.inputSince)
	}
	got := timeline(t, a)
	if !strings.Contains(got, "00:10-06:33 Idle") || !strings.Contains(got, "09:00-10:00 PDF Reading") {
		t.Fatalf("timeline:\n%s", got)
	}
	// Unhooked again: it starts over when hooked next.
	if a.loadInputSince("linux", false) != math.MaxInt64 || a.loadInputSince("linux", true) != apptest.Now.UnixMilli() {
		t.Fatal("input since not reset")
	}
}

func TestStaleRemoteMachine(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:00", End: "14:10", Bundle: "term", App: "WezTerm", Title: "x"})
	dir := t.TempDir()
	e.WriteConfig(t, "[sync]\ndir = '"+dir+"'\nmachine = 'mac'\n")
	write := func(machine, body string) {
		p := filepath.Join(dir, machine, "2026-10-10.jsonl")
		os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	line := func(from, to string) string {
		return `{"s":` + strconv.FormatInt(apptest.At(t, from, 0), 10) + `,"e":` + strconv.FormatInt(apptest.At(t, to, 0), 10) +
			`,"b":"pdf","a":"zathura","t":"book","c":"Reading"}` + "\n"
	}
	write("ryuk", line("01:00", "04:00"))
	write("box", line("05:00", "08:00"))
	os.WriteFile(filepath.Join(dir, "box", syncdir.MachineFile), []byte(`{"input":true}`), 0o600)
	a := open(t, e)
	if _, err := a.Sync(1); err != nil {
		t.Fatal(err)
	}
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	// ryuk says nothing about input: 10 min; box reports input: 3h; and
	// 10 min here.
	if d.TrackedMS != 3*3_600_000+20*60_000 {
		t.Fatalf("tracked = %d", d.TrackedMS)
	}
	if d.Cut.StaleMS != 3*3_600_000-10*60_000 {
		t.Fatalf("cut = %+v", d.Cut)
	}
	b, err := os.ReadFile(filepath.Join(dir, "mac", syncdir.MachineFile))
	if err != nil || !strings.HasPrefix(string(b), `{"input":true,"hid":true,"rules_at":`) {
		t.Fatalf("machine.json = %q, %v", b, err)
	}
}

func TestHypridleConfigs(t *testing.T) {
	proc := t.TempDir()
	for pid, cmd := range map[string]string{
		"12":   "hypridle\x00-c\x00/home/x/.config/ryoku/hypridle.conf\x00",
		"13":   "/usr/bin/hypridle\x00--config=/etc/hi.conf\x00",
		"14":   "bash\x00-c\x00hypridle\x00",
		"15":   "/home/x/.local/bin/tock-hypridle\x00-c\x00/home/x/.config/tock/hypridle.conf\x00",
		"self": "hypridle\x00-c\x00/nope\x00",
	} {
		os.MkdirAll(filepath.Join(proc, pid), 0o700)
		os.WriteFile(filepath.Join(proc, pid, "cmdline"), []byte(cmd), 0o600)
	}
	got, _ := hypridleConfigs(proc)
	slices.Sort(got)
	if want := []string{"/etc/hi.conf", "/home/x/.config/ryoku/hypridle.conf", "/home/x/.config/tock/hypridle.conf"}; !slices.Equal(got, want) {
		t.Fatalf("configs = %v", got)
	}
}

func TestStaleRemoteBeforeInputBegan(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:00", End: "14:10", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, syncdir.MachineFile), []byte(`{"input":true,"input_since":`+
		strconv.FormatInt(apptest.At(t, "08:00", 0), 10)+"}\n"), 0o600)
	if err := a.readMachineFile(dir, "ryuk"); err != nil {
		t.Fatal(err)
	}
	rv := []store.RemoteView{
		{Machine: "ryuk", StartMS: apptest.At(t, "00:00", 0), EndMS: apptest.At(t, "06:33", 0), Bundle: "pdf", App: "PDF", Category: "Reading"},
		{Machine: "ryuk", StartMS: apptest.At(t, "09:00", 0), EndMS: apptest.At(t, "10:00", 0), Bundle: "pdf", App: "PDF", Category: "Reading"},
	}
	got := a.staleRemote(rv)
	if got[0].EndMS != apptest.At(t, "00:10", 0) || got[1].EndMS != apptest.At(t, "10:00", 0) {
		t.Fatalf("cut = %v, %v", time.UnixMilli(got[0].EndMS), time.UnixMilli(got[1].EndMS))
	}
}

func TestHypridleHookedNeedsARunningHypridle(t *testing.T) {
	home, proc := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	conf := filepath.Join(home, ".config", "hypr", "hypridle.conf")
	os.MkdirAll(filepath.Dir(conf), 0o700)
	os.WriteFile(conf, []byte("listener {\n  on-timeout = trackd idle\n}\n"), 0o600)
	if hypridleHooked(proc, home) {
		t.Fatal("hooked with no hypridle running")
	}
	os.MkdirAll(filepath.Join(proc, "40"), 0o700)
	os.WriteFile(filepath.Join(proc, "40", "cmdline"), []byte("hypridle\x00"), 0o600)
	if !hypridleHooked(proc, home) {
		t.Fatal("a hypridle on the default config is not hooked")
	}
}

func TestSyncNoticesHypridleStop(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:00", End: "14:10", Bundle: "term", App: "WezTerm", Title: "x"})
	dir := t.TempDir()
	e.WriteConfig(t, "[sync]\ndir = '"+dir+"'\nmachine = 'ryuk'\n")
	a := open(t, e)
	hooked := true
	a.goos, a.inputCheck = "linux", func() bool { return hooked }
	a.refreshInput()
	if !a.input {
		t.Fatal("no input with hypridle hooked")
	}
	hooked = false
	if _, err := a.Sync(1); err != nil {
		t.Fatal(err)
	}
	if a.input || a.inputSince != math.MaxInt64 {
		t.Fatalf("input = %v since %d after hypridle stopped", a.input, a.inputSince)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ryuk", syncdir.MachineFile))
	if !strings.HasPrefix(string(b), `{"input":false`) {
		t.Fatalf("machine.json = %q", b)
	}
}

func TestTrustRemoteWithoutHID(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:00", End: "14:10", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	for machine, body := range map[string]string{"ryuk": `{"input":true}`, "air": `{"input":true,"hid":true}`} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, syncdir.MachineFile), []byte(body), 0o600)
		if err := a.readMachineFile(dir, machine); err != nil {
			t.Fatal(err)
		}
	}
	rv := []store.RemoteView{
		{Machine: "ryuk", StartMS: apptest.At(t, "00:00", 0), EndMS: apptest.At(t, "09:51", 0), Bundle: "pdf", App: "PDF", Category: "Reading"},
		{Machine: "ryuk", StartMS: apptest.At(t, "10:00", 0), EndMS: apptest.At(t, "12:00", 0), Bundle: "pdf", App: "PDF", Category: "Reading"},
		{Machine: "air", StartMS: apptest.At(t, "00:00", 0), EndMS: apptest.At(t, "09:51", 0), Bundle: "pdf", App: "PDF", Category: "Reading"},
	}
	got := a.staleRemote(rv)
	// ryuk's 9h51 is past [afk] trust (3h): its 10 min window; 2h stays; the
	// Mac-like machine is trusted.
	if got[0].EndMS != apptest.At(t, "00:10", 0) || got[1].EndMS != apptest.At(t, "12:00", 0) || got[2].EndMS != apptest.At(t, "09:51", 0) {
		t.Fatalf("cut = %v, %v, %v", time.UnixMilli(got[0].EndMS), time.UnixMilli(got[1].EndMS), time.UnixMilli(got[2].EndMS))
	}
}

func TestTrustLocalLinux(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "00:30", End: "09:30", Bundle: "pdf", App: "zathura", Title: "book"})
	a := open(t, e)
	a.goos, a.inputCheck = "linux", func() bool { return true }
	a.refreshInput()
	a.inputSince = math.MinInt64 + 1 // input all along
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	if d.TrackedMS != 10*60_000 {
		t.Fatalf("tracked = %d", d.TrackedMS)
	}
}

func TestCutToOtherMachine(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:00", End: "15:00", Bundle: "term", App: "WezTerm", Title: "x"})
	dir := t.TempDir()
	e.WriteConfig(t, "[sync]\ndir = '"+dir+"'\nmachine = 'mac'\n")
	os.MkdirAll(filepath.Join(dir, "ryuk"), 0o700)
	os.WriteFile(filepath.Join(dir, "ryuk", "2026-10-10.jsonl"), []byte(`{"s":`+strconv.FormatInt(apptest.At(t, "14:30", 0), 10)+
		`,"e":`+strconv.FormatInt(apptest.At(t, "14:45", 0), 10)+`,"b":"pdf","a":"zathura","t":"book","c":"Reading"}`+"\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "ryuk", syncdir.MachineFile), []byte(`{"input":true}`), 0o600)
	a := open(t, e)
	if _, err := a.Sync(1); err != nil {
		t.Fatal(err)
	}
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	// ryuk turned a page at 14:30: its 15 min are its own, the hour here
	// keeps 45 min, and nothing is lost.
	if d.Cut.OtherMS != 15*60_000 || d.TrackedMS != 60*60_000 {
		t.Fatalf("cut = %+v, tracked = %d", d.Cut, d.TrackedMS)
	}
}

func TestMachineFileCarriesVersionAndUpdateError(t *testing.T) {
	e := apptest.New(t, afkRules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:00", End: "14:10", Bundle: "term", App: "WezTerm", Title: "x"})
	dir := t.TempDir()
	a := open(t, e)
	// none known: both left out
	if err := a.writeMachineFile(dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, syncdir.MachineFile)); strings.Contains(string(b), "version") || strings.Contains(string(b), "update_error") {
		t.Fatalf("machine.json = %q", b)
	}
	a.st.SetState(store.KeyVersion, "v1", 1)
	a.st.SetState(update.KeyError, "update: key changed", 1)
	if err := a.writeMachineFile(dir); err != nil {
		t.Fatal(err)
	}
	mi, ok := syncdir.ReadMachineInfo(dir)
	if !ok || mi.Version != "v1" || mi.UpdateError != "update: key changed" {
		t.Fatalf("machine info = %+v", mi)
	}
	if err := a.readMachineFile(dir, "ryuk"); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := a.st.State(RemoteVersionKey("ryuk")); v != "v1" {
		t.Fatalf("remote version = %q", v)
	}
	if v, _, _ := a.st.State(RemoteUpdateErrorKey("ryuk")); v != "update: key changed" {
		t.Fatalf("remote update error = %q", v)
	}
	// a machine.json without them clears both
	os.WriteFile(filepath.Join(dir, syncdir.MachineFile), []byte(`{"input":true}`), 0o600)
	if err := a.readMachineFile(dir, "ryuk"); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := a.st.State(RemoteVersionKey("ryuk")); v != "" {
		t.Fatalf("remote version = %q after it went", v)
	}
	if v, _, _ := a.st.State(RemoteUpdateErrorKey("ryuk")); v != "" {
		t.Fatalf("remote update error = %q after it went", v)
	}
}
