package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"

	"tock/internal/classify"
	"tock/internal/store"
)

// machineFile is the file in a machine's sync folder that says what the
// machine reports, so the others can tell whether its time saw input.
const machineFile = "machine.json"

type machineInfo struct {
	Input    bool   `json:"input"`
	RulesAt  int64  `json:"rules_at,omitempty"`  // rules.toml's change time, ms
	RulesSum string `json:"rules_sum,omitempty"` // sha256 of the shared copy
}

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

// HypridleHooked reports whether ~/.config/hypr/hypridle.conf (or
// $XDG_CONFIG_HOME's) runs trackd idle.
func HypridleHooked(home string) bool {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(home, ".config")
	}
	b, err := os.ReadFile(filepath.Join(dir, "hypr", "hypridle.conf"))
	return err == nil && strings.Contains(string(b), "trackd idle")
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
	if !a.input {
		afk.StaleMS = a.cfg.AFK.StaleMS
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
	known := map[string]bool{}
	out := make([]store.RemoteView, 0, len(rv))
	for _, r := range rv {
		in, ok := known[r.Machine]
		if !ok {
			v, _, _ := a.st.State(RemoteInputKey(r.Machine))
			in = v == "1"
			known[r.Machine] = in
		}
		if !in && r.EndMS-r.StartMS > a.cfg.AFK.StaleMS {
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
	v := "0"
	if mi.Input {
		v = "1"
	}
	return a.st.SetState(RemoteInputKey(machine), v, a.nowMS())
}
