package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// Every process with tock.db open holds a shared flock on tock.db.lock and
// a holders/<pid> file naming itself. tock restore takes the lock
// exclusively, so it never swaps the file under a reader or writer, and
// reads the holder files to say who is in the way.

// Holder is a process that has the database open.
type Holder struct {
	PID  int
	Name string
}

// HeldError is returned by Exclusive while other processes hold the store.
type HeldError struct{ Holders []Holder }

func (e *HeldError) Error() string {
	if len(e.Holders) == 0 {
		return "another process has tock.db open; close tock web and any running tock first"
	}
	parts := make([]string, len(e.Holders))
	for i, h := range e.Holders {
		parts[i] = fmt.Sprintf("%s (pid %d)", h.Name, h.PID)
	}
	return "close these first: " + strings.Join(parts, ", ")
}

func lockPath(dbPath string) string { return dbPath + ".lock" }

func holdersDir(dbPath string) string { return filepath.Join(filepath.Dir(dbPath), "holders") }

// openLock opens (creating) the lock file and flocks it with how.
func openLock(dbPath string, how int) (*os.File, error) {
	f, err := os.OpenFile(lockPath(dbPath), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: lock file: %w", err)
	}
	for {
		err = syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

var (
	holdMu    sync.Mutex
	holdCount = map[string]int{} // db path -> stores open in this process
)

// shareLock takes the shared lock for a store being opened and records this
// process as a holder. The returned func undoes both.
func shareLock(dbPath string) (func(), error) {
	f, err := openLock(dbPath, syscall.LOCK_SH)
	if err != nil {
		return nil, fmt.Errorf("store: shared lock: %w", err)
	}
	key := filepath.Clean(dbPath)
	holdMu.Lock()
	if holdCount[key] == 0 {
		writeHolder(dbPath)
	}
	holdCount[key]++
	holdMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			holdMu.Lock()
			holdCount[key]--
			if holdCount[key] == 0 {
				delete(holdCount, key)
				_ = os.Remove(filepath.Join(holdersDir(dbPath), strconv.Itoa(os.Getpid())))
			}
			holdMu.Unlock()
			_ = f.Close() // closing the descriptor drops the flock
		})
	}, nil
}

// writeHolder is best effort: the lock works without it, the refusal
// message is only less specific.
func writeHolder(dbPath string) {
	dir := holdersDir(dbPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, strconv.Itoa(os.Getpid())), []byte(processName()), 0o600)
}

// processName is the program and its subcommand: "tock web", "trackd".
func processName() string {
	name := filepath.Base(os.Args[0])
	if len(os.Args) > 1 && os.Args[1] != "" && !strings.HasPrefix(os.Args[1], "-") {
		name += " " + os.Args[1]
	}
	return name
}

// holders lists the live processes that recorded themselves for dbPath and
// removes the files of dead ones.
func holders(dbPath string) []Holder {
	dir := holdersDir(dbPath)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Holder
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			_ = os.Remove(p)
			continue
		}
		b, _ := os.ReadFile(p)
		name := strings.TrimSpace(string(b))
		if name == "" {
			name = "unknown"
		}
		out = append(out, Holder{PID: pid, Name: name})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].PID < out[b].PID })
	return out
}

// Exclusive takes the database lock exclusively without waiting. It fails
// with a *HeldError while any store is open on dbPath. The returned func
// releases the lock.
func Exclusive(dbPath string) (func(), error) {
	f, err := openLock(dbPath, syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, &HeldError{Holders: holders(dbPath)}
	}
	if err != nil {
		return nil, fmt.Errorf("store: exclusive lock: %w", err)
	}
	return func() { _ = f.Close() }, nil
}
