package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tock/internal/classify"
	"tock/internal/store"
)

// machineFile is the file in a machine's sync folder that says what the
// machine reports, so the others can tell whether its time saw input.
const machineFile = "machine.json"

type machineInfo struct {
	Input      bool   `json:"input"`
	InputSince int64  `json:"input_since,omitempty"` // when it began to report input, ms; 0 for always
	RulesAt    int64  `json:"rules_at,omitempty"`    // rules.toml's change time, ms
	RulesSum   string `json:"rules_sum,omitempty"`   // sha256 of the shared copy
}

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

// RemoteInputSinceKey is the daemon_state key holding when a remote
// machine began reporting input (ms), "" or absent for always.
func RemoteInputSinceKey(machine string) string { return "remote_input_since:" + machine }

// RemoteInputKey is the daemon_state key holding "1" when a remote machine
// says it reports input.
func RemoteInputKey(machine string) string { return "remote_input:" + machine }

// inputKnown reports whether this machine reports input to trackd: always
// on macOS; on Linux only once hypridle runs trackd idle.
func inputKnown(goos, home string) bool {
	if goos != "linux" {
		return true
	}
	return HypridleHooked(home)
}

// HypridleHooked reports whether hypridle's config runs trackd idle: the
// config of each running hypridle (its -c or --config), else
// ~/.config/hypr/hypridle.conf (or $XDG_CONFIG_HOME's).
func HypridleHooked(home string) bool {
	paths := hypridleConfigs("/proc")
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
// alone).
func hypridleConfigs(proc string) []string {
	ents, err := os.ReadDir(proc)
	if err != nil {
		return nil
	}
	var out []string
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
		for i, a := range args {
			switch {
			case (a == "-c" || a == "--config") && i+1 < len(args):
				out = append(out, args[i+1])
			case strings.HasPrefix(a, "--config="):
				out = append(out, strings.TrimPrefix(a, "--config="))
			}
		}
	}
	return out
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

// base turns rows into this machine's views over [from, to): unclipped
// first, so a pause across the edges is judged whole, with shell dirs and
// the AFK windows applied, then clipped. The open span ends at nowMS.
func (a *App) base(rows []store.SpanRow, ovs map[int64]classify.Override, from, to, nowMS int64) []classify.SpanView {
	vs := views(rows, math.MinInt64, math.MaxInt64, nowMS)
	a.attachShell(vs)
	afk := classify.AFK{Window: a.afkWindow(ovs)}
	if a.inputSince > math.MinInt64 {
		afk.StaleMS, afk.StaleUntil = a.cfg.AFK.StaleMS, a.inputSince
	}
	return clipViews(classify.MarkAFK(vs, afk), from, to)
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
// category's window.
func (a *App) staleRemote(rv []store.RemoteView) []store.RemoteView {
	// since is when each machine began reporting input: MaxInt64 for not.
	since := map[string]int64{}
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
		}
		if r.StartMS < s && r.EndMS-r.StartMS > a.cfg.AFK.StaleMS {
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
	mi := machineInfo{Input: a.input}
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
	_, err = writeIfChanged(filepath.Join(dir, machineFile), append(b, '\n'))
	return err
}

// readMachineFile records what a remote machine's machine.json says.
func (a *App) readMachineFile(dir, machine string) error {
	mi, ok := readMachineInfo(dir)
	if !ok {
		return nil // an older tock writes none: unknown
	}
	v, since := "0", ""
	if mi.Input {
		v = "1"
	}
	if mi.InputSince > 0 {
		since = strconv.FormatInt(mi.InputSince, 10)
	}
	if err := a.st.SetState(RemoteInputSinceKey(machine), since, a.nowMS()); err != nil {
		return err
	}
	return a.st.SetState(RemoteInputKey(machine), v, a.nowMS())
}
