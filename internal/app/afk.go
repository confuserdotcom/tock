package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"tock/internal/classify"
	"tock/internal/span"
	"tock/internal/store"
	"tock/internal/syncdir"
	"tock/internal/update"
)

// KeyInputSince is the daemon_state key holding when this Linux machine
// began reporting input (ms), "" while it does not.
const KeyInputSince = "input_since"

// loadInputSince is when this machine began reporting input (see
// App.inputSince), recording now as that time the first time it does.
func (a *App) loadInputSince(goos string, input bool) int64 {
	switch {
	case goos != "linux":
		return math.MinInt64
	case !input:
		a.st.SetState(KeyInputSince, "", a.nowMS())
		return math.MaxInt64
	}
	v, _, _ := a.st.State(KeyInputSince)
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	now := a.nowMS()
	a.st.SetState(KeyInputSince, strconv.FormatInt(now, 10), now)
	return now
}

// refreshInput checks again whether this machine reports input, so a
// long-running trackd notices hypridle stop and start; input_since moves to
// when it came back.
func (a *App) refreshInput() {
	a.input = a.inputCheck()
	a.inputSince = a.loadInputSince(a.goos, a.input)
}

// RemoteInputSinceKey is the daemon_state key holding when a remote
// machine began reporting input (ms), "" or absent for always.
func RemoteInputSinceKey(machine string) string { return "remote_input_since:" + machine }

// RemoteHIDKey is the daemon_state key holding "1" when a remote machine
// says its input comes from the OS, which never misses idle.
func RemoteHIDKey(machine string) string { return "remote_hid:" + machine }

// RemoteInputKey is the daemon_state key holding "1" when a remote machine
// says it reports input.
func RemoteInputKey(machine string) string { return "remote_input:" + machine }

// RemoteVersionKey is the daemon_state key holding a remote machine's trackd
// version, "" when it does not say.
func RemoteVersionKey(machine string) string { return "remote_version:" + machine }

// RemoteUpdateErrorKey is the daemon_state key holding a remote machine's
// last failed self-update, "" when none.
func RemoteUpdateErrorKey(machine string) string { return "remote_update_error:" + machine }

// KeyRulesRefused is the daemon_state key holding the newer rules.toml this
// machine last refused ("<machine>: why"), and RemoteRulesRefusedKey the
// same for another machine, from its machine.json.
const KeyRulesRefused = "rules_refused"

func RemoteRulesRefusedKey(machine string) string { return "remote_rules_refused:" + machine }

// inputKnown reports whether this machine reports input to trackd: always
// on macOS; on Linux only once hypridle runs trackd idle.
func inputKnown(goos, home string) bool {
	if goos != "linux" {
		return true
	}
	return HypridleHooked(home)
}

// HypridleHooked reports whether a running hypridle's config runs trackd
// idle: its -c or --config, else ~/.config/hypr/hypridle.conf (or
// $XDG_CONFIG_HOME's). With no hypridle running, nothing reports input.
func HypridleHooked(home string) bool { return hypridleHooked("/proc", home) }

func hypridleHooked(proc, home string) bool {
	paths, running := hypridleConfigs(proc)
	if !running {
		return false
	}
	if len(paths) == 0 {
		dir := os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		paths = []string{filepath.Join(dir, "hypr", "hypridle.conf")}
	}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), "trackd idle") {
			return true
		}
	}
	return false
}

// hypridleConfigs is the config file of each hypridle running under proc
// that names one with -c or --config, including one started under another
// name (a tock-hypridle link, so a desktop that manages hypridle leaves it
// alone), and whether any hypridle runs at all.
func hypridleConfigs(proc string) (out []string, running bool) {
	ents, err := os.ReadDir(proc)
	if err != nil {
		return nil, false
	}
	for _, e := range ents {
		if e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		b, err := os.ReadFile(filepath.Join(proc, e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
		if !strings.Contains(filepath.Base(args[0]), "hypridle") { // also a renamed one, like tock-hypridle
			continue
		}
		running = true
		for i, a := range args {
			switch {
			case (a == "-c" || a == "--config") && i+1 < len(args):
				out = append(out, args[i+1])
			case strings.HasPrefix(a, "--config="):
				out = append(out, strings.TrimPrefix(a, "--config="))
			}
		}
	}
	return out, running
}

// InputKnown reports whether this machine's time saw input (see inputKnown).
func (a *App) InputKnown() bool { return a.input }

// afkWindow is the AFK window of a view: its category's afk.
func (a *App) afkWindow(ovs map[int64]classify.Override) func(classify.SpanView) int64 {
	return func(v classify.SpanView) int64 {
		var o *classify.Override
		if ov, ok := ovs[v.ID]; ok {
			o = &ov
		}
		return a.rs.AFKWindow(a.rs.Label(v, o).Category)
	}
}

// base turns rows into this machine's views, whole, so a pause across
// [from, to)'s edges is judged whole and classify.Resolve sees when each
// began, with shell dirs and the AFK windows applied. Resolve clips them.
// The open span ends at nowMS.
func (a *App) base(rows []store.SpanRow, ovs map[int64]classify.Override, from, to, nowMS int64) []classify.SpanView {
	vs := views(rows, math.MinInt64, math.MaxInt64, nowMS)
	a.attachShell(vs)
	afk := classify.AFK{Window: a.afkWindow(ovs)}
	if a.inputSince > math.MinInt64 {
		afk.StaleMS, afk.StaleUntil = a.cfg.AFK.StaleMS, a.inputSince
	}
	if a.goos == "linux" {
		afk.TrustMS = a.cfg.AFK.TrustMS
	}
	out := classify.MarkAFK(vs, afk)
	idle := map[int64]bool{}
	for _, r := range rows {
		idle[r.ID] = r.BundleID == span.IdleBundleID
	}
	for _, v := range clipViews(out, from, to) {
		if v.BundleID == span.IdleBundleID && v.ID > 0 && !idle[v.ID] {
			a.cut.StaleMS += v.DurMS()
		}
	}
	return out
}

// rowEnds is, for rows in start order, the latest end among rows 0..i (the
// open span ends at nowMS): it never decreases, so rowsFor can search it.
func rowEnds(rows []store.SpanRow, nowMS int64) []int64 {
	out := make([]int64, len(rows))
	var m int64 = math.MinInt64
	for i, r := range rows {
		end := nowMS
		if r.End != nil {
			end = *r.End
		}
		m = max(m, end)
		out[i] = m
	}
	return out
}

// rowsFor is the rows base needs for [from, to), out of rows in start
// order with their rowEnds: those that start before to and reach past
// classify.MaxAFKMS before from, where a pause into the day is judged.
// One day's base then costs that day's rows, not the whole range's.
func rowsFor(rows []store.SpanRow, ends []int64, from, to int64) []store.SpanRow {
	lo := sort.Search(len(ends), func(i int) bool { return ends[i] > from-classify.MaxAFKMS })
	hi := sort.Search(len(rows), func(i int) bool { return rows[i].Start >= to })
	if hi < lo {
		return nil
	}
	return rows[lo:hi]
}

// clipViews cuts views to [from, to) and drops the ones left empty.
func clipViews(vs []classify.SpanView, from, to int64) []classify.SpanView {
	out := vs[:0:0]
	for _, v := range vs {
		v.Start, v.End = max(v.Start, from), min(v.End, to)
		if v.End > v.Start {
			out = append(out, v)
		}
	}
	return out
}

// staleRemote applies the stale rule to the views of machines that do not
// say they report input: a view longer than [afk] stale keeps its
// category's window. So does a view longer than [afk] trust on a machine
// whose input does not come from the OS.
func (a *App) staleRemote(rv []store.RemoteView) []store.RemoteView {
	// since is when each machine began reporting input: MaxInt64 for not.
	since := map[string]int64{}
	hid := map[string]bool{}
	out := make([]store.RemoteView, 0, len(rv))
	for _, r := range rv {
		s, ok := since[r.Machine]
		if !ok {
			s = math.MaxInt64
			if v, _, _ := a.st.State(RemoteInputKey(r.Machine)); v == "1" {
				w, _, _ := a.st.State(RemoteInputSinceKey(r.Machine))
				s, _ = strconv.ParseInt(w, 10, 64) // "" is 0: always
			}
			since[r.Machine] = s
			v, _, _ := a.st.State(RemoteHIDKey(r.Machine))
			hid[r.Machine] = v == "1"
		}
		dur := r.EndMS - r.StartMS
		if (r.StartMS < s && dur > a.cfg.AFK.StaleMS) || (!hid[r.Machine] && dur > a.cfg.AFK.TrustMS) {
			cat := r.Category
			if !a.rs.HasCategory(cat) {
				cat = a.rs.Label(classify.SpanView{BundleID: r.Bundle, App: r.App, Title: r.Title, URL: r.URL}, nil).Category
			}
			if cut := r.StartMS + a.rs.AFKWindow(cat); cut < r.EndMS {
				r.EndMS = cut
			}
		}
		out = append(out, r)
	}
	return out
}

// writeMachineFile writes this machine's machine.json to its sync folder,
// and shares rules.toml unless [sync] rules is off.
func (a *App) writeMachineFile(dir string) error {
	mi := syncdir.MachineInfo{Input: a.input, HID: a.goos != "linux"}
	mi.Version, _, _ = a.st.State(store.KeyVersion)
	mi.UpdateError, _, _ = a.st.State(update.KeyError)
	mi.RulesRefused, _, _ = a.st.State(KeyRulesRefused)
	if a.input && a.inputSince > math.MinInt64 {
		mi.InputSince = a.inputSince
	}
	if !a.cfg.Sync.NoRules {
		if err := a.shareRules(dir, &mi); err != nil {
			return err
		}
	}
	b, err := json.Marshal(mi)
	if err != nil {
		return err
	}
	_, err = syncdir.WriteIfChanged(filepath.Join(dir, syncdir.MachineFile), append(b, '\n'))
	return err
}

// readMachineFile records what a remote machine's machine.json says.
func (a *App) readMachineFile(dir, machine string) error {
	mi, ok := syncdir.ReadMachineInfo(dir)
	if !ok {
		return nil // an older tock writes none: unknown
	}
	v, since, hid := "0", "", "0"
	if mi.Input {
		v = "1"
	}
	if mi.HID {
		hid = "1"
	}
	if err := a.st.SetState(RemoteHIDKey(machine), hid, a.nowMS()); err != nil {
		return err
	}
	if mi.InputSince > 0 {
		since = strconv.FormatInt(mi.InputSince, 10)
	}
	if err := a.st.SetState(RemoteInputSinceKey(machine), since, a.nowMS()); err != nil {
		return err
	}
	if err := a.st.SetState(RemoteVersionKey(machine), mi.Version, a.nowMS()); err != nil {
		return err
	}
	if err := a.st.SetState(RemoteUpdateErrorKey(machine), mi.UpdateError, a.nowMS()); err != nil {
		return err
	}
	if err := a.st.SetState(RemoteRulesRefusedKey(machine), mi.RulesRefused, a.nowMS()); err != nil {
		return err
	}
	return a.st.SetState(RemoteInputKey(machine), v, a.nowMS())
}
