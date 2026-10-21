// Package update publishes tock's Linux builds to the sync folder and
// installs them on Linux.
//
// The Mac signs a manifest of sha256 sums with an ed25519 key and writes it
// last, next to the builds. A Linux machine pins the key the first time it
// sees it, and installs a build only when the signature and every sum hold.
package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	// BinDir is the folder in the [sync] dir that holds the published builds.
	BinDir = "bin"
	// PubFile is the public key beside the builds.
	PubFile = "update.pub"
	// ManifestFile lists a build's version and the sha256 of its files.
	ManifestFile = "manifest.json"
	// SigFile is the signature of ManifestFile.
	SigFile = "manifest.sig"
)

const (
	// KeyError is the daemon_state key holding the last update error, ""
	// when the last check was fine.
	KeyError = "update_error"
	// KeyApplied is the daemon_state key holding "<version> <manifest
	// sha256>" of the last install, written before the files are replaced.
	KeyApplied = "update_applied"
)

// Names are the files in a build, and the only ones ever installed.
// They are renamed in this order, so trackd comes last.
var Names = []string{"tock", "trackd"}

// ErrKeyChanged means the key offered in the sync folder is not the one
// pinned on this machine.
var ErrKeyChanged = errors.New("update: key changed")

var sumRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ArchDir is the folder of a build for arch.
func ArchDir(arch string) string { return "linux-" + arch }

func validArch(arch string) bool { return arch == "amd64" || arch == "arm64" }

// Manifest says what a published build is.
type Manifest struct {
	Version string            `json:"version"`
	BuiltAt time.Time         `json:"built_at"`
	Files   map[string]string `json:"files"` // name to sha256 hex
}

// LoadOrCreateKey reads the signing key at path, making it (0600, in a 0700
// dir) when it does not exist. It refuses a key others can read.
func LoadOrCreateKey(path string) (ed25519.PrivateKey, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return createKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("update: key: %w", err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("update: key %s is readable by others; chmod 600 it", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("update: key: %w", err)
	}
	raw, err := decode(b, ed25519.PrivateKeySize)
	if err != nil {
		return nil, fmt.Errorf("update: key %s: %w", path, err)
	}
	return ed25519.PrivateKey(raw), nil
}

func createKey(path string) (ed25519.PrivateKey, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("update: key: %w", err)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("update: key: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("update: key: %w", err)
	}
	_, err = f.Write(encode(key))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("update: key: %w", err)
	}
	return key, nil
}

// encode is one line of base64 text.
func encode(b []byte) []byte { return []byte(base64.StdEncoding.EncodeToString(b) + "\n") }

// decode reads one line of base64 text that must hold n bytes.
func decode(b []byte, n int) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("not base64: %w", err)
	}
	if len(raw) != n {
		return nil, fmt.Errorf("holds %d bytes, want %d", len(raw), n)
	}
	return raw, nil
}

// EncodePub is the text of a public key file.
func EncodePub(pub ed25519.PublicKey) []byte { return encode(pub) }

// Sign is the text of manifest.sig for manifest.
func Sign(key ed25519.PrivateKey, manifest []byte) []byte {
	return encode(ed25519.Sign(key, manifest))
}

// Publish writes a build for arch into binDir: the files of src, then
// manifest.sig, then manifest.json last, so a reader that finds the manifest
// finds the rest. Every file is written beside its name and renamed.
func Publish(binDir, arch, src, version string, builtAt time.Time, key ed25519.PrivateKey) error {
	switch {
	case !validArch(arch):
		return fmt.Errorf("update: arch %q, want amd64 or arm64", arch)
	case version == "":
		return errors.New("update: empty version")
	case builtAt.IsZero():
		return errors.New("update: zero built_at")
	}
	dir := filepath.Join(binDir, ArchDir(arch))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("update: publish: %w", err)
	}
	m := Manifest{Version: version, BuiltAt: builtAt.UTC(), Files: map[string]string{}}
	for _, name := range Names {
		s, err := copyFile(filepath.Join(src, name), dir, name)
		if err != nil {
			return fmt.Errorf("update: publish %s: %w", name, err)
		}
		m.Files[name] = s
	}
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("update: publish: %w", err)
	}
	mb = append(mb, '\n')
	if err := writeFile(filepath.Join(dir, SigFile), Sign(key, mb), false); err != nil {
		return fmt.Errorf("update: publish: %w", err)
	}
	if err := writeFile(filepath.Join(dir, ManifestFile), mb, false); err != nil {
		return fmt.Errorf("update: publish: %w", err)
	}
	if err := writeFile(filepath.Join(binDir, PubFile), EncodePub(key.Public().(ed25519.PublicKey)), true); err != nil {
		return fmt.Errorf("update: publish: %w", err)
	}
	return nil
}

// copyFile copies src to dir/name through a tmp file and returns the sha256
// of what it wrote.
func copyFile(src, dir, name string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	tmp := filepath.Join(dir, "."+name+".tmp")
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeFile replaces path with b through a tmp file. With skip it leaves a
// file that already holds b alone, so a sync tool sees no change.
func writeFile(path string, b []byte, skip bool) error {
	if old, err := os.ReadFile(path); skip && err == nil && bytes.Equal(old, b) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Verify checks sig against the exact bytes of manifest, then that the
// manifest names a version and exactly the files in Names, each with a
// sha256 sum.
func Verify(manifest, sig []byte, pub ed25519.PublicKey) (Manifest, error) {
	raw, err := decode(sig, ed25519.SignatureSize)
	if err != nil || !ed25519.Verify(pub, manifest, raw) {
		return Manifest{}, errors.New("update: manifest.sig does not match manifest.json (still syncing, or not signed by the pinned key)")
	}
	var m Manifest
	if err := json.Unmarshal(manifest, &m); err != nil {
		return Manifest{}, fmt.Errorf("update: manifest: %w", err)
	}
	if m.Version == "" || m.BuiltAt.IsZero() {
		return Manifest{}, errors.New("update: manifest has no version or built_at")
	}
	if len(m.Files) != len(Names) {
		return Manifest{}, fmt.Errorf("update: manifest names %d files, want %v", len(m.Files), Names)
	}
	for _, name := range Names {
		if !sumRE.MatchString(m.Files[name]) {
			return Manifest{}, fmt.Errorf("update: manifest has no sha256 for %s", name)
		}
	}
	return m, nil
}

// Pin is trust on first use. pinned is this machine's copy of the key and
// offered the one in the sync folder. A missing pinned key takes the offered
// one; a different offered key is refused with ErrKeyChanged and pinned stays.
func Pin(pinned, offered string) (ed25519.PublicKey, error) {
	have, perr := os.ReadFile(pinned)
	if perr != nil && !errors.Is(perr, os.ErrNotExist) {
		return nil, fmt.Errorf("update: pinned key: %w", perr)
	}
	got, oerr := os.ReadFile(offered)
	if oerr != nil && !errors.Is(oerr, os.ErrNotExist) {
		return nil, fmt.Errorf("update: key: %w", oerr)
	}
	switch {
	case perr == nil && oerr == nil && !bytes.Equal(bytes.TrimSpace(have), bytes.TrimSpace(got)):
		return nil, fmt.Errorf("%w: %s differs from the pinned %s; delete the pinned key to trust it", ErrKeyChanged, offered, pinned)
	case perr == nil:
		return parsePub(have, pinned)
	case oerr != nil:
		return nil, fmt.Errorf("update: no key at %s yet (still syncing?)", offered)
	}
	pub, err := parsePub(got, offered)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(pinned), 0o700); err != nil {
		return nil, fmt.Errorf("update: pin: %w", err)
	}
	tmp := filepath.Join(filepath.Dir(pinned), "."+filepath.Base(pinned)+".tmp")
	if err := os.WriteFile(tmp, EncodePub(pub), 0o644); err != nil {
		return nil, fmt.Errorf("update: pin: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err == nil {
		err = os.Rename(tmp, pinned)
	}
	if err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("update: pin: %w", err)
	}
	return pub, nil
}

func parsePub(b []byte, path string) (ed25519.PublicKey, error) {
	raw, err := decode(b, ed25519.PublicKeySize)
	if err != nil {
		return nil, fmt.Errorf("update: key %s: %w", path, err)
	}
	return ed25519.PublicKey(raw), nil
}

// Published is the newest manifest in binDir. It is for display: it does not
// check the signature.
func Published(binDir string) (Manifest, bool) {
	paths, _ := filepath.Glob(filepath.Join(binDir, "linux-*", ManifestFile))
	slices.Sort(paths)
	var best Manifest
	found := false
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(b, &m) != nil || m.Version == "" || m.BuiltAt.IsZero() {
			continue
		}
		if !found || m.BuiltAt.After(best.BuiltAt) {
			best, found = m, true
		}
	}
	return best, found
}

// BootstrapCommand is the one command for a Linux machine that runs a tock
// from before updates: it installs the published build and the unit. It
// copies to .new and renames, as a running trackd cannot be overwritten, and
// runs under bash because the login shell may be fish.
func BootstrapCommand(syncDir, home, arch string) string {
	dir := syncDir
	if home != "" {
		if rel, err := filepath.Rel(home, syncDir); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, "../") {
			dir = "~/" + rel
		}
	}
	return "bash -c 'mkdir -p ~/.local/bin && cd ~/.local/bin && for f in trackd tock; do cp " +
		dir + "/" + BinDir + "/" + ArchDir(arch) + "/$f $f.new && chmod 755 $f.new && mv -f $f.new $f || exit 1; done && ./trackd install'"
}
