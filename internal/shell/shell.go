// Package shell reads the log the zsh hook writes (tock shell-init zsh) and
// attaches working directories to terminal views. Each line is
// "<unix ms>\t<absolute dir>".
package shell

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"tock/internal/classify"
)

// File is the log's name in tock's state directory.
const File = "shell.log"

// Event is one hook call: at TS the shell's working directory was Dir.
type Event struct {
	TS  int64
	Dir string
}

// Log reads a shell log and caches what it parsed: a grown file is read
// from where the last read stopped, a shrunk one from the start. It is not
// safe for concurrent use.
type Log struct {
	Path   string
	size   int64
	mod    time.Time
	off    int64 // bytes parsed, always just after a newline
	events []Event
}

// Events returns every event sorted by TS. A missing file has none.
func (l *Log) Events() ([]Event, error) {
	fi, err := os.Stat(l.Path)
	if errors.Is(err, os.ErrNotExist) {
		l.size, l.mod, l.off, l.events = 0, time.Time{}, 0, nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if fi.Size() == l.size && fi.ModTime().Equal(l.mod) {
		return l.events, nil
	}
	if fi.Size() < l.off {
		l.off, l.events = 0, nil
	}
	f, err := os.Open(l.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(l.off, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	end := bytes.LastIndexByte(b, '\n') + 1 // a partial last line waits for the next read
	sorted := true
	for _, line := range strings.Split(string(b[:end]), "\n") {
		e, ok := parse(line)
		if !ok {
			continue
		}
		if n := len(l.events); n > 0 && e.TS < l.events[n-1].TS {
			sorted = false
		}
		l.events = append(l.events, e)
	}
	if !sorted {
		sort.SliceStable(l.events, func(i, j int) bool { return l.events[i].TS < l.events[j].TS })
	}
	l.off += int64(end)
	l.size, l.mod = fi.Size(), fi.ModTime()
	return l.events, nil
}

func parse(line string) (Event, bool) {
	ts, dir, ok := strings.Cut(line, "\t")
	if !ok || !strings.HasPrefix(dir, "/") {
		return Event{}, false
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return Event{}, false
	}
	return Event{TS: n, Dir: dir}, true
}

// Attach sets Dir on every view whose bundle is in terminals: the dir that
// covers most of the view, the first to reach that time on a tie. An event
// covers [TS, the next event's TS). A view with no event before its end gets
// no dir. Dirs under home are written "~/…". ev must be sorted by TS.
func Attach(views []classify.SpanView, ev []Event, terminals []string, home string) {
	if len(ev) == 0 {
		return
	}
	for i := range views {
		v := &views[i]
		if !slices.Contains(terminals, v.BundleID) {
			continue
		}
		// start at the last event at or before the view's start
		j := max(sort.Search(len(ev), func(k int) bool { return ev[k].TS > v.Start })-1, 0)
		best, bestMS := "", int64(0)
		cover := map[string]int64{}
		for ; j < len(ev) && ev[j].TS < v.End; j++ {
			from, to := max(ev[j].TS, v.Start), v.End
			if j+1 < len(ev) {
				to = min(to, ev[j+1].TS)
			}
			if to <= from {
				continue
			}
			cover[ev[j].Dir] += to - from
			if c := cover[ev[j].Dir]; c > bestMS {
				best, bestMS = ev[j].Dir, c
			}
		}
		if bestMS == 0 {
			continue
		}
		d := fold(best, home)
		v.Dir = &d
	}
}

// fold writes home, and paths under it, with "~".
func fold(dir, home string) string {
	home = strings.TrimRight(home, "/")
	switch {
	case home == "":
		return dir
	case dir == home:
		return "~"
	case strings.HasPrefix(dir, home+"/"):
		return "~" + dir[len(home):]
	}
	return dir
}
