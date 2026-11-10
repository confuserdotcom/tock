package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Running is the build that is running now.
type Running struct {
	Version string
	BuiltAt time.Time // zero when unknown (a dev build)
}

// ParseRunning reads the version and the RFC 3339 build time baked in with
// -ldflags. An empty or bad time gives a zero BuiltAt.
func ParseRunning(version, builtAt string) Running {
	t, _ := time.Parse(time.RFC3339, builtAt)
	return Running{Version: version, BuiltAt: t}
}

// Plan is a verified build that can be installed.
type Plan struct {
	Version string
	BuiltAt time.Time
	Sum     string            // sha256 of the manifest bytes
	Dir     string            // where the build's files are
	Files   map[string]string // name to sha256 hex
}

// Mark is what is recorded as installed (KeyApplied) before the files are
// replaced.
func (p Plan) Mark() string { return p.Version + " " + p.Sum }

// Check looks in binDir for a build for arch that is newer than run and
// signed by the pinned key. It returns false for no build, the same build or
// an older one; an error when the build cannot be trusted yet, which a later
// call may clear (Syncthing does not keep the order of its files). applied
// is KeyApplied: a manifest installed once is not offered again.
func Check(binDir, arch, pinned string, run Running, applied string) (Plan, bool, error) {
	dir := filepath.Join(binDir, ArchDir(arch))
	mb, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if errors.Is(err, os.ErrNotExist) {
		return Plan{}, false, nil
	}
	if err != nil {
		return Plan{}, false, fmt.Errorf("update: %w", err)
	}
	var head Manifest
	if err := json.Unmarshal(mb, &head); err != nil {
		return Plan{}, false, fmt.Errorf("update: %s: %w (still syncing?)", ManifestFile, err)
	}
	if head.Version == run.Version {
		return Plan{}, false, nil
	}
	pub, err := Pinned(pinned, filepath.Join(binDir, PubFile))
	if err != nil {
		return Plan{}, false, err
	}
	sig, err := os.ReadFile(filepath.Join(dir, SigFile))
	if err != nil {
		return Plan{}, false, fmt.Errorf("update: %s: %w (still syncing?)", SigFile, err)
	}
	m, err := Verify(mb, sig, pub)
	if err != nil {
		return Plan{}, false, err
	}
	if !run.BuiltAt.IsZero() && !m.BuiltAt.After(run.BuiltAt) {
		return Plan{}, false, nil // never a downgrade or a replay
	}
	s := sha256.Sum256(mb)
	p := Plan{Version: m.Version, BuiltAt: m.BuiltAt, Sum: hex.EncodeToString(s[:]), Dir: dir, Files: m.Files}
	if p.Mark() == applied {
		return Plan{}, false, fmt.Errorf("update: %s was installed, yet trackd still runs %s", m.Version, run.Version)
	}
	return p, true, nil
}

// rename is os.Rename; tests make it fail.
var rename = os.Rename

// Apply installs the files of p into binDir. Each is copied to name.new and
// hashed as it is written, so the bytes that are renamed are the ones that
// were checked. mark runs once, after every copy is staged and before the
// first rename. Each installed file is kept as name.old until every file is
// in place, so any error, a failed rename included, leaves binDir as it was.
func Apply(p Plan, binDir string, mark func() error) error {
	var staged []string
	cleanup := func() {
		for _, s := range staged {
			os.Remove(s)
		}
	}
	for _, name := range Names {
		tmp := filepath.Join(binDir, name+".new")
		staged = append(staged, tmp)
		if err := stage(filepath.Join(p.Dir, name), tmp, p.Files[name]); err != nil {
			cleanup()
			return fmt.Errorf("update: %s: %w", name, err)
		}
	}
	if err := mark(); err != nil {
		cleanup()
		return fmt.Errorf("update: record: %w", err)
	}
	var kept []string // files moved to .old, to put back on failure
	var placed []string
	rollback := func() {
		for _, dst := range placed {
			os.Remove(dst)
		}
		for _, dst := range kept {
			_ = rename(dst+".old", dst)
		}
		cleanup()
	}
	for i, name := range Names {
		dst := filepath.Join(binDir, name)
		if _, err := os.Lstat(dst); err == nil {
			if err := rename(dst, dst+".old"); err != nil {
				rollback()
				return fmt.Errorf("update: keep %s: %w", name, err)
			}
			kept = append(kept, dst)
		}
		if err := rename(staged[i], dst); err != nil {
			rollback()
			return fmt.Errorf("update: install %s: %w", name, err)
		}
		placed = append(placed, dst)
	}
	for _, dst := range kept {
		os.Remove(dst + ".old")
	}
	return nil
}

// stage copies src to dst and checks the bytes written against want.
func stage(src, dst, want string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if err == nil {
		err = out.Chmod(0o755) // trackd runs under umask 077
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("does not match the manifest (still syncing?)")
	}
	return nil
}
