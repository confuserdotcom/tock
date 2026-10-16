package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/store"
)

// SyncDays is how many days, today included, each sync writes.
const SyncDays = 2

// dayFileRE is a day file's name in a machine's sync folder.
var dayFileRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.jsonl$`)

// syncRecord is one line of a day file: a view of this machine and its
// label here.
type syncRecord struct {
	S int64   `json:"s"`
	E int64   `json:"e"`
	B string  `json:"b"`
	A string  `json:"a"`
	T *string `json:"t,omitempty"`
	U *string `json:"u,omitempty"`
	C string  `json:"c,omitempty"`
	P string  `json:"p,omitempty"`
}

// SyncReport says what one sync did.
type SyncReport struct {
	Machine  string
	Written  int // day files written (unchanged ones are skipped)
	Read     int // other machines' day files read
	Machines []store.RemoteMachine
	// RulesFrom is the machine whose newer rules.toml this sync took;
	// RulesRefused a newer copy it could not take, as "<machine>: why".
	RulesFrom, RulesRefused string
}

// ErrSyncOff means config.toml has no [sync] dir.
var ErrSyncOff = errors.New("sync is off: set [sync] dir in config.toml")

// Sync writes this machine's last days days to the sync folder and reads
// the other machines' changed day files.
func (a *App) Sync(days int) (SyncReport, error) {
	r := SyncReport{Machine: a.cfg.Sync.MachineName()}
	dir := a.cfg.Sync.Dir
	if dir == "" {
		return r, ErrSyncOff
	}
	if !a.cfg.Sync.NoRules {
		var err error
		if r.RulesFrom, r.RulesRefused, err = a.takeRules(dir, r.Machine); err != nil {
			return r, err
		}
	}
	today := a.midnight(a.now())
	for i := range max(days, 1) {
		day := today.AddDate(0, 0, -i)
		b, err := a.exportDay(day)
		if err != nil {
			return r, err
		}
		wrote, err := writeIfChanged(filepath.Join(dir, r.Machine, dateKey(day)+".jsonl"), b)
		if err != nil {
			return r, err
		}
		if wrote {
			r.Written++
		}
	}
	if err := a.writeMachineFile(filepath.Join(dir, r.Machine)); err != nil {
		return r, err
	}
	n, err := a.importRemote(dir, r.Machine)
	r.Read = n
	if err != nil {
		return r, err
	}
	r.Machines, err = a.st.RemoteMachines()
	return r, err
}

// exportDay is the day file of day: this machine's activity (never idle,
// synthetic or other machines' views), with its labels here.
func (a *App) exportDay(day time.Time) ([]byte, error) {
	from := a.midnight(day)
	to := from.AddDate(0, 0, 1)
	vs, labels, err := a.labelled(from.UnixMilli(), to.UnixMilli(), a.nowMS())
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	for i, v := range vs {
		if v.ID <= 0 || strings.HasPrefix(v.BundleID, "tock.") || v.End <= v.Start {
			continue
		}
		rec := syncRecord{S: v.Start, E: v.End, B: v.BundleID, A: v.App, T: v.Title, U: v.URL,
			C: labels[i].Category, P: labels[i].Project}
		if v.Private {
			rec.T, rec.U = nil, nil
		}
		if err := enc.Encode(rec); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

// writeIfChanged replaces path with b unless it already holds b. The new
// file is written beside it and renamed, so a syncing tool never sees half
// a file.
func writeIfChanged(path string, b []byte) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, err
	}
	return true, nil
}

// importRemote reads every other machine's day files that changed since
// they were last read.
func (a *App) importRemote(dir, self string) (int, error) {
	machines, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("sync dir: %w", err)
	}
	read := 0
	for _, m := range machines {
		if !m.IsDir() || m.Name() == self || strings.HasPrefix(m.Name(), ".") {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, m.Name()))
		if err != nil {
			return read, err
		}
		if err := a.readMachineFile(filepath.Join(dir, m.Name()), m.Name()); err != nil {
			return read, err
		}
		for _, f := range files {
			match := dayFileRE.FindStringSubmatch(f.Name())
			if match == nil {
				continue
			}
			fi, err := f.Info()
			if err != nil {
				continue // removed meanwhile
			}
			cur := store.RemoteFile{MtimeMS: fi.ModTime().UnixMilli(), Size: fi.Size()}
			seen, ok, err := a.st.RemoteFileSeen(m.Name(), match[1])
			if err != nil {
				return read, err
			}
			if ok && seen == cur {
				continue
			}
			vs, err := readDayFile(filepath.Join(dir, m.Name(), f.Name()))
			if err != nil {
				return read, err
			}
			if err := a.st.ReplaceRemoteDay(m.Name(), match[1], cur, vs); err != nil {
				return read, err
			}
			read++
		}
	}
	return read, nil
}

// readDayFile parses a day file. Lines that do not parse are skipped: a
// newer tock may write more.
func readDayFile(path string) ([]store.RemoteView, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []store.RemoteView
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var r syncRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.E <= r.S || r.B == "" {
			continue
		}
		out = append(out, store.RemoteView{StartMS: r.S, EndMS: r.E, Bundle: r.B, App: r.A, Title: r.T, URL: r.U,
			Category: r.C, Project: r.P})
	}
	return out, sc.Err()
}

// IsRemoteView reports whether id is another machine's view.
func IsRemoteView(id int64) bool {
	_, ok := classify.RemoteRowID(id)
	return ok
}
