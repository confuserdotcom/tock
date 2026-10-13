package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"time"

	"tock/internal/shell"
	"tock/internal/store"
)

// ErrPruneOff means [privacy] keep_titles is not set and no days were given.
var ErrPruneOff = errors.New("title deletion is off: set [privacy] keep_titles in config.toml")

// PruneReport says what one title deletion did, or would do.
type PruneReport struct {
	Before time.Time // local midnight: spans that ended before it are pruned
	Spans  int       // this machine's spans
	Remote int       // other machines' views
	Files  int       // this machine's day files in the sync folder
}

// PruneTitles deletes window titles, and cuts URLs to their host, for the
// activity that ended before local midnight days days ago (days <= 0: the
// [privacy] keep_titles setting). Each span keeps the label its rules gave
// it, so no label changes. Other machines' views and this machine's own
// day files in the sync folder are pruned the same way. dryRun only counts
// this machine's spans and day files.
func (a *App) PruneTitles(days int, dryRun bool) (PruneReport, error) {
	if days <= 0 {
		days = a.cfg.Privacy.KeepTitles
	}
	if days <= 0 {
		return PruneReport{}, ErrPruneOff
	}
	r := PruneReport{Before: a.midnight(a.now()).AddDate(0, 0, -days)}
	before := r.Before.UnixMilli()
	rows, err := a.st.TitledBefore(before)
	if err != nil {
		return r, err
	}
	r.Spans = len(rows)
	if len(rows) > 0 && !dryRun {
		if err := a.st.PruneSpans(a.keptLabels(rows)); err != nil {
			return r, err
		}
	}
	if !dryRun {
		if r.Remote, err = a.st.PruneRemote(before); err != nil {
			return r, err
		}
	}
	if dir := a.cfg.Sync.Dir; dir != "" {
		if r.Files, err = pruneDayFiles(filepath.Join(dir, a.cfg.Sync.MachineName()), r.Before, dryRun); err != nil {
			return r, err
		}
	}
	return r, nil
}

// keptLabels is the rule label of each row, with its shell dir attached as
// the views see it, ignoring calls and manual labels.
func (a *App) keptLabels(rows []store.SpanRow) []store.Kept {
	vs := views(rows, math.MinInt64, math.MaxInt64, a.nowMS())
	if a.paths.ShellLog != "" {
		if ev, err := a.shell.Events(); err == nil {
			shell.Attach(vs, ev, a.cfg.Shell.Terminals, a.home)
		}
	}
	out := make([]store.Kept, len(vs))
	for i, v := range vs {
		l := a.rs.Label(v, nil)
		out[i] = store.Kept{SpanID: v.ID, Category: l.Category, Project: l.Project}
	}
	return out
}

// pruneDayFiles rewrites the day files in dir dated before before without
// titles and with URLs cut to their host. It returns how many changed (or
// would change, with dryRun).
func pruneDayFiles(dir string, before time.Time, dryRun bool) (int, error) {
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	cut := before.Format("2006-01-02")
	n := 0
	for _, f := range files {
		m := dayFileRE.FindStringSubmatch(f.Name())
		if m == nil || m[1] >= cut {
			continue
		}
		path := filepath.Join(dir, f.Name())
		old, err := os.ReadFile(path)
		if err != nil {
			return n, err
		}
		b, changed := pruneDayFile(old)
		if !changed {
			continue
		}
		n++
		if dryRun {
			continue
		}
		if _, err := writeIfChanged(path, b); err != nil {
			return n, err
		}
	}
	return n, nil
}

// pruneDayFile drops the titles and URL paths of a day file's records.
// Lines that do not parse are kept as they are.
func pruneDayFile(b []byte) ([]byte, bool) {
	var out bytes.Buffer
	changed := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var r syncRecord
		if json.Unmarshal(line, &r) != nil {
			out.Write(line)
			out.WriteByte('\n')
			continue
		}
		if r.T != nil {
			r.T, changed = nil, true
		}
		if r.U != nil {
			if h := store.URLHost(*r.U); h != *r.U {
				r.U, changed = &h, true
			}
		}
		enc, _ := json.Marshal(r)
		out.Write(enc)
		out.WriteByte('\n')
	}
	return out.Bytes(), changed
}
