package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tock/internal/classify"
)

// rulesCopy is this machine's rules.toml in its sync folder. machine.json
// says when it last changed and its checksum, so a machine takes a newer
// copy only once the whole of it has arrived.
const rulesCopy = "rules.toml"

// KeyRulesFrom is the daemon_state key naming the machine whose rules.toml
// this machine last took, as "<machine> <changed ms>".
const KeyRulesFrom = "rules_from"

func rulesSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// localRules is this machine's rules.toml and when it last changed.
func (a *App) localRules() ([]byte, int64, error) {
	b, err := os.ReadFile(a.paths.Rules)
	if err != nil {
		return nil, 0, err
	}
	fi, err := os.Stat(a.paths.Rules)
	if err != nil {
		return nil, 0, err
	}
	return b, fi.ModTime().UnixMilli(), nil
}

// shareRules copies rules.toml to this machine's sync folder and fills in
// what machine.json says about it.
func (a *App) shareRules(dir string, mi *machineInfo) error {
	b, at, err := a.localRules()
	if err != nil {
		return err
	}
	if _, err := writeIfChanged(filepath.Join(dir, rulesCopy), b); err != nil {
		return err
	}
	mi.RulesAt, mi.RulesSum = at, rulesSum(b)
	return nil
}

// takeRules replaces rules.toml with the newest other machine's copy when
// that copy changed after this one, arrived whole, differs and parses with
// this machine's learned.toml. The file it replaces is kept as
// rules.toml.bak and the new one keeps the other machine's change time, so
// it is not passed back as newer. It returns the machine taken from, or a
// copy it could not take and why.
func (a *App) takeRules(dir, self string) (from, refused string, err error) {
	local, localAt, err := a.localRules()
	if err != nil {
		return "", "", err
	}
	learned, err := os.ReadFile(a.paths.Learned)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", fmt.Errorf("sync dir: %w", err)
	}
	var best []byte
	bestAt := localAt
	for _, e := range entries {
		if !e.IsDir() || e.Name() == self || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		mdir := filepath.Join(dir, e.Name())
		mi, ok := readMachineInfo(mdir)
		if !ok || mi.RulesSum == "" || mi.RulesAt <= bestAt {
			continue
		}
		b, err := os.ReadFile(filepath.Join(mdir, rulesCopy))
		if err != nil || rulesSum(b) != mi.RulesSum || bytes.Equal(b, local) {
			continue // still arriving, or the same rules
		}
		if _, err := classify.Parse(b, learned); err != nil {
			refused = e.Name() + ": " + err.Error()
			continue
		}
		best, bestAt, from = b, mi.RulesAt, e.Name()
	}
	if best == nil {
		return "", refused, nil
	}
	if err := os.WriteFile(a.paths.Rules+".bak", local, 0o600); err != nil {
		return "", "", err
	}
	if err := classify.WriteFile(a.paths.Rules, best); err != nil {
		return "", "", err
	}
	at := time.UnixMilli(bestAt)
	if err := os.Chtimes(a.paths.Rules, at, at); err != nil {
		return "", "", err
	}
	if err := a.st.SetState(KeyRulesFrom, from+" "+strconv.FormatInt(bestAt, 10), a.nowMS()); err != nil {
		return "", "", err
	}
	_, err = a.Reload()
	return from, "", err
}

// RulesFrom is the machine whose rules.toml this machine last took and when
// that copy had changed; "" when none.
func (a *App) RulesFrom() (string, time.Time) {
	v, _, _ := a.st.State(KeyRulesFrom)
	name, ms, ok := strings.Cut(v, " ")
	n, err := strconv.ParseInt(ms, 10, 64)
	if !ok || err != nil {
		return "", time.Time{}
	}
	return name, time.UnixMilli(n)
}

// readMachineInfo reads a machine folder's machine.json.
func readMachineInfo(dir string) (machineInfo, bool) {
	b, err := os.ReadFile(filepath.Join(dir, machineFile))
	if err != nil {
		return machineInfo{}, false
	}
	var mi machineInfo
	if json.Unmarshal(b, &mi) != nil {
		return machineInfo{}, false
	}
	return mi, true
}
