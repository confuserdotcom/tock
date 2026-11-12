package app

import (
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"os"
	"runtime/debug"
	"time"

	"tock/internal/classify"
	"tock/internal/store"
)

// pastFormat is the version of what the disk cache of past days holds and
// how it is computed: bump it when pastDay, or the code that fills it,
// changes in a way the build revision does not show.
const pastFormat = 3

// diskDay is a past day as the derived table keeps it.
type diskDay struct {
	FromMS, ToMS int64
	Day          pastDay
}

// dayKeys is the disk key of each day: a hash of everything its result is
// computed from, so a cached day is used only while every input is the
// same. The inputs are the rules, the learned rules, the config, the shell
// log's head, the code, and what touches the day or the pause judged at
// its start (classify.MaxAFKMS before it): overrides and kept labels of its
// spans, away rows, calls, events, edits, plan slots and the other
// machines' views with what they say about their input. The day's spans
// themselves are not read: a day goes to disk only once closed (see
// fillPast), and a pruned title shows as a kept label. Nor is the local
// model: a closed day keeps the model fills it was computed with, as the
// model is trained again every modelEveryMS. nil when an input cannot be
// read.
func (a *App) dayKeys(days []time.Time) map[string]string {
	if len(days) == 0 {
		return nil
	}
	global, ok := a.inputsHash()
	if !ok {
		return nil
	}

	lo, hi := days[0].UnixMilli()-classify.MaxAFKMS, days[len(days)-1].AddDate(0, 0, 1).UnixMilli()
	now := a.nowMS()
	marks, err := a.st.SpanMarks(lo, hi, now)
	if err != nil {
		return nil
	}
	aw, err := a.st.Aways(lo, hi)
	if err != nil {
		return nil
	}
	cs, err := a.st.Calls(lo, hi)
	if err != nil {
		return nil
	}
	evs, err := a.st.Events(lo, hi)
	if err != nil {
		return nil
	}
	eds, err := a.st.Edits(lo, hi)
	if err != nil {
		return nil
	}
	rv, err := a.st.RemoteViews(lo, hi)
	if err != nil {
		return nil
	}
	slots := a.planSlots(lo, hi)
	machines := map[string]string{}
	for _, r := range rv {
		if _, ok := machines[r.Machine]; !ok {
			in, _, _ := a.st.State(RemoteInputKey(r.Machine))
			since, _, _ := a.st.State(RemoteInputSinceKey(r.Machine))
			hid, _, _ := a.st.State(RemoteHIDKey(r.Machine))
			machines[r.Machine] = in + "/" + since + "/" + hid
		}
	}

	str := func(p *string) string {
		if p == nil {
			return "\x01"
		}
		return *p
	}
	out := make(map[string]string, len(days))
	for _, d := range days {
		f, t := d.UnixMilli()-classify.MaxAFKMS, d.AddDate(0, 0, 1).UnixMilli()
		in := func(s, e int64) bool { return s < t && e > f }
		h := fnv.New64a()
		fmt.Fprintf(h, "%x\x00%s\x00%d\x00%d\x00", global, dateKey(d), d.UnixMilli(), t)
		for _, m := range marks {
			if in(m.StartMS, m.EndMS) {
				fmt.Fprintf(h, "m%d %v %q %q\x00", m.SpanID, m.Kept, m.Category, m.Project)
			}
		}
		for _, r := range aw {
			if in(r.StartMS, r.EndMS) {
				fmt.Fprintf(h, "a%d %d %d %q %q\x00", r.ID, r.StartMS, r.EndMS, r.Category, r.Project)
			}
		}
		for _, c := range cs {
			if in(c.StartMS, c.EndMS) {
				fmt.Fprintf(h, "c%d %d\x00", c.StartMS, c.EndMS)
			}
		}
		for _, e := range evs {
			if in(e.StartMS, e.EndMS) {
				fmt.Fprintf(h, "e%d %d %d %q\x00", e.ID, e.StartMS, e.EndMS, e.Title)
			}
		}
		for _, e := range eds {
			if in(e.StartMS, e.EndMS) {
				fmt.Fprintf(h, "d%d %s %d %d %q %q %q\x00", e.ID, e.Kind, e.StartMS, e.EndMS, e.Category, e.Project, e.Note)
			}
		}
		for _, s := range slots {
			if in(s.From, s.To) {
				fmt.Fprintf(h, "p%d %d %q\x00", s.From, s.To, s.Project)
			}
		}
		remoteKey(h, rv, machines, in, str)
		out[dateKey(d)] = fmt.Sprintf("%016x", h.Sum64())
	}
	return out
}

func remoteKey(h hash.Hash64, rv []store.RemoteView, machines map[string]string, in func(s, e int64) bool, str func(*string) string) {
	for _, r := range rv {
		if in(r.StartMS, r.EndMS) {
			fmt.Fprintf(h, "r%d %s %s %d %d %q %q %q %q %q %q\x00", r.ID, r.Machine, machines[r.Machine], r.StartMS, r.EndMS,
				r.Bundle, r.App, str(r.Title), str(r.URL), r.Category, r.Project)
		}
	}
}

// inputsHash is a hash of what every derived result depends on besides the
// database: the code, the rules, the learned rules, the config and the
// shell log's head. false when a file cannot be read.
func (a *App) inputsHash() (uint64, bool) {
	g := fnv.New64a()
	fmt.Fprintf(g, "%d\x00%s\x00%s\x00%d\x00", pastFormat, buildID(), a.goos, a.inputSince)
	for _, p := range []string{a.paths.Rules, a.paths.Learned, a.paths.Config} {
		b, err := os.ReadFile(p)
		if err != nil && !os.IsNotExist(err) {
			return 0, false
		}
		g.Write(b)
		g.Write([]byte{0})
	}
	// Shell dirs of past views come from the log's old lines: a log cut at
	// its head changes them.
	if a.paths.ShellLog != "" {
		if f, err := os.Open(a.paths.ShellLog); err == nil {
			_, _ = io.CopyN(g, f, 4096)
			f.Close()
		}
	}
	return g.Sum64(), true
}

// buildID names the code that computed a cached day: the VCS revision, and
// for a build with local changes the binary's time too.
func buildID() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value
		}
	}
	if dirty == "true" {
		if exe, err := os.Executable(); err == nil {
			if fi, err := os.Stat(exe); err == nil {
				rev += "+" + fi.ModTime().UTC().Format(time.RFC3339Nano)
			}
		}
	}
	return rev
}

// pastName is the derived row of a past day. Days labelled with agent
// sessions (tock) and without (trackd) are kept apart, so neither process
// overwrites the other's.
func (a *App) pastName(date string) string {
	if a.agentLabels() {
		return date + "+agents"
	}
	return date
}

// loadPast is the days on disk whose key is still keys[day].
func (a *App) loadPast(keys map[string]string) map[string]pastDay {
	out := map[string]pastDay{}
	names := make([]string, 0, len(keys))
	for d := range keys {
		names = append(names, a.pastName(d))
	}
	rows, err := a.st.DerivedRows(names)
	if err != nil {
		return out
	}
	loc := a.now().Location()
	for date := range keys {
		c, ok := rows[a.pastName(date)]
		if !ok {
			continue
		}
		var d diskDay
		day, err := time.ParseInLocation("2006-01-02", date, loc)
		if err != nil || c.Key != keys[date] || json.Unmarshal(c.Data, &d) != nil ||
			d.FromMS != day.UnixMilli() || d.ToMS != day.AddDate(0, 0, 1).UnixMilli() {
			continue
		}
		out[date] = d.Day
	}
	return out
}

// savePast writes closed days to disk under their keys. A cache: a failed
// write only costs the next run the time to compute them again.
func (a *App) savePast(days map[string]pastDay, keys map[string]string) {
	rows := make(map[string]store.Derived, len(days))
	loc := a.now().Location()
	for date, p := range days {
		day, err := time.ParseInLocation("2006-01-02", date, loc)
		if err != nil || keys[date] == "" {
			continue
		}
		b, err := json.Marshal(diskDay{FromMS: day.UnixMilli(), ToMS: day.AddDate(0, 0, 1).UnixMilli(), Day: p})
		if err == nil {
			rows[a.pastName(date)] = store.Derived{Key: keys[date], Data: b}
		}
	}
	if len(rows) > 0 {
		_ = a.st.PutDerived(rows, a.nowMS())
	}
}
