// Package syncdir reads and writes the files of the sync folder: each
// machine's day files, its machine.json and its copy of rules.toml. What to
// put in them, and what to do with what the other machines wrote, is the
// app's.
package syncdir

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"tock/internal/store"
)

// DayFileRE is a day file's name in a machine's sync folder.
var DayFileRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.jsonl$`)

// Record is one line of a day file: a view of this machine and its
// label here.
type Record struct {
	S int64   `json:"s"`
	E int64   `json:"e"`
	B string  `json:"b"`
	A string  `json:"a"`
	T *string `json:"t,omitempty"`
	U *string `json:"u,omitempty"`
	C string  `json:"c,omitempty"`
	P string  `json:"p,omitempty"`
}

// WriteIfChanged replaces path with b unless it already holds b. The new
// file is written beside it and renamed, so a syncing tool never sees half
// a file.
func WriteIfChanged(path string, b []byte) (bool, error) {
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

// ReadDayFile parses a day file. Lines that do not parse are skipped: a
// newer tock may write more.
func ReadDayFile(path string) ([]store.RemoteView, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []store.RemoteView
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.E <= r.S || r.B == "" {
			continue
		}
		out = append(out, store.RemoteView{StartMS: r.S, EndMS: r.E, Bundle: r.B, App: r.A, Title: r.T, URL: r.U,
			Category: r.C, Project: r.P})
	}
	return out, sc.Err()
}

// MachineFile is the file in a machine's sync folder that says what the
// machine reports, so the others can tell whether its time saw input.
const MachineFile = "machine.json"

// MachineInfo is what machine.json holds.
type MachineInfo struct {
	Input      bool   `json:"input"`
	HID        bool   `json:"hid,omitempty"`         // input comes from the OS (macOS), not hypridle
	InputSince int64  `json:"input_since,omitempty"` // when it began to report input, ms; 0 for always
	RulesAt    int64  `json:"rules_at,omitempty"`    // rules.toml's change time, ms
	RulesSum   string `json:"rules_sum,omitempty"`   // sha256 of the shared copy
	// Version is this machine's trackd version; UpdateError its last failed
	// self-update.
	Version     string `json:"version,omitempty"`
	UpdateError string `json:"update_error,omitempty"`
	// RulesRefused is the newer rules.toml this machine last could not
	// take, as "<machine>: why"; "" when it took or had nothing to take.
	RulesRefused string `json:"rules_refused,omitempty"`
}

// ReadMachineInfo reads a machine folder's machine.json.
func ReadMachineInfo(dir string) (MachineInfo, bool) {
	b, err := os.ReadFile(filepath.Join(dir, MachineFile))
	if err != nil {
		return MachineInfo{}, false
	}
	var mi MachineInfo
	if json.Unmarshal(b, &mi) != nil {
		return MachineInfo{}, false
	}
	return mi, true
}

// RulesCopy is this machine's rules.toml in its sync folder. machine.json
// says when it last changed and its checksum, so a machine takes a newer
// copy only once the whole of it has arrived.
const RulesCopy = "rules.toml"

// RulesSum is the checksum machine.json gives for a rules copy.
func RulesSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
