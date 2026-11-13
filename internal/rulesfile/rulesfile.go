// Package rulesfile is the only writer of rules.toml and learned.toml. Every
// change goes the same way: under one lock across processes, checked to
// parse, each changed file kept as .bak, learned.toml before rules.toml,
// each file replaced atomically.
package rulesfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tock/internal/classify"
	"tock/internal/flock"
)

// lockFile, in the rules' directory, serializes every read-modify-write of
// the rule files across processes: tock commands, the TUI, tock web and
// trackd's sync could otherwise each read the files, change them and lose
// the other's change.
const lockFile = ".rules.lock"

// lockWait is how long a writer waits for another to finish.
const lockWait = 10 * time.Second

// ErrChanged means a file is not as the caller read it: someone else
// changed it since.
var ErrChanged = errors.New("rulesfile: the file changed since it was read")

// Files are the paths of the two rule files.
type Files struct {
	Rules, Learned string
}

// Set is the content of both files. HasLearned is false when learned.toml
// does not exist.
type Set struct {
	Rules, Learned []byte
	HasLearned     bool
}

// Options are what some writes need besides the content.
type Options struct {
	// At, when not zero, becomes rules.toml's change time (a copy taken
	// from another machine keeps that machine's time).
	At time.Time
	// BeforeRules runs once learned.toml is written and before rules.toml
	// is: the database is brought to the new names there, so a crash
	// leaves rules.toml as it was and the next try does it all again.
	BeforeRules func() error
}

// Lock runs fn holding the rule files' lock. Read and Write are called
// inside it; Lock is not reentrant.
func (f Files) Lock(fn func() error) error {
	path := filepath.Join(filepath.Dir(f.Rules), lockFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("rulesfile: lock: %w", err)
	}
	lf, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("rulesfile: lock: %w", err)
	}
	defer lf.Close() // closing releases the lock
	deadline := time.Now().Add(lockWait)
	for {
		err := flock.Lock(lf, false, false)
		if err == nil {
			break
		}
		if !errors.Is(err, flock.ErrHeld) || time.Now().After(deadline) {
			return fmt.Errorf("rulesfile: lock: another tock is changing the rules: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fn()
}

// Read reads both files. rules.toml must exist; learned.toml may not.
func (f Files) Read() (Set, error) {
	rules, err := os.ReadFile(f.Rules)
	if err != nil {
		return Set{}, fmt.Errorf("rulesfile: read: %w", err)
	}
	learned, err := os.ReadFile(f.Learned)
	if errors.Is(err, os.ErrNotExist) {
		return Set{Rules: rules}, nil
	}
	if err != nil {
		return Set{}, fmt.Errorf("rulesfile: read: %w", err)
	}
	return Set{Rules: rules, Learned: learned, HasLearned: true}, nil
}

// Write replaces prev, as read under the same Lock, with next. next must
// parse; a file that changed is kept as .bak first. Nothing is written
// when the files are no longer prev (ErrChanged) or next does not parse.
func (f Files) Write(prev, next Set, o Options) error {
	cur, err := f.Read()
	if err != nil {
		return err
	}
	if !bytes.Equal(cur.Rules, prev.Rules) || !bytes.Equal(cur.Learned, prev.Learned) || cur.HasLearned != prev.HasLearned {
		return ErrChanged
	}
	if _, err := classify.Parse(next.Rules, next.Learned); err != nil {
		return fmt.Errorf("rulesfile: the new rules do not parse (%v); nothing changed", err)
	}
	learnedChanged := !bytes.Equal(next.Learned, prev.Learned) && (prev.HasLearned || len(next.Learned) > 0)
	if learnedChanged {
		if prev.HasLearned {
			if err := os.WriteFile(f.Learned+".bak", prev.Learned, 0o600); err != nil {
				return fmt.Errorf("rulesfile: keep learned.toml: %w", err)
			}
		}
		if err := classify.WriteFile(f.Learned, next.Learned); err != nil {
			return fmt.Errorf("rulesfile: write learned.toml: %w", err)
		}
	}
	if o.BeforeRules != nil {
		if err := o.BeforeRules(); err != nil {
			return err
		}
	}
	if !bytes.Equal(next.Rules, prev.Rules) {
		if err := os.WriteFile(f.Rules+".bak", prev.Rules, 0o600); err != nil {
			return fmt.Errorf("rulesfile: keep rules.toml: %w", err)
		}
		if err := classify.WriteFile(f.Rules, next.Rules); err != nil {
			return fmt.Errorf("rulesfile: write rules.toml: %w", err)
		}
	}
	if !o.At.IsZero() {
		if err := os.Chtimes(f.Rules, o.At, o.At); err != nil {
			return fmt.Errorf("rulesfile: set rules.toml's time: %w", err)
		}
	}
	return nil
}

// Rewrite reads both files under the lock, hands them to edit and writes
// what it returns.
func (f Files) Rewrite(edit func(cur Set) (Set, error), o Options) error {
	return f.Lock(func() error {
		cur, err := f.Read()
		if err != nil {
			return err
		}
		next, err := edit(cur)
		if err != nil {
			return err
		}
		return f.Write(cur, next, o)
	})
}

// Restore puts path (one of the two files) back as an undo kept it:
// before when it existed, removed when not. after is what the change left;
// a file that is not after any more is someone else's since (ErrChanged).
// Call it inside Lock.
func (f Files) Restore(path string, existed bool, before, after []byte) error {
	if path != f.Rules && path != f.Learned {
		return fmt.Errorf("rulesfile: restore %s: not a rule file", path)
	}
	cur, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rulesfile: restore: %w", err)
	}
	if !bytes.Equal(cur, after) {
		return ErrChanged
	}
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("rulesfile: restore: %w", err)
		}
		return nil
	}
	if err := classify.WriteFile(path, before); err != nil {
		return fmt.Errorf("rulesfile: restore: %w", err)
	}
	return nil
}
