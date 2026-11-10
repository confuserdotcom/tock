// Command tockpub publishes tock's Linux builds to the sync folder, signed.
// make publish-linux runs it; it is not installed.
package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tock/internal/config"
	"tock/internal/update"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "tockpub: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tockpub", flag.ContinueOnError)
	version := fs.String("version", "", "version of the build (required)")
	builtAt := fs.String("built-at", "", "build time, RFC 3339 (required)")
	syncDir := fs.String("sync", "", "sync folder (default: [sync] dir in config.toml)")
	keyPath := fs.String("key", filepath.Join(dataDir(), "update.key"), "signing key, made if missing")
	hint := fs.Bool("hint", false, "print the one-time command for a Linux machine and exit")
	arch := fs.String("arch", "amd64", "the Linux machine's architecture, for -hint")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *syncDir == "" {
		c, err := config.Load(filepath.Join(configDir(), config.File))
		if err != nil {
			return err
		}
		*syncDir = c.Sync.Dir
	}
	if *syncDir == "" {
		return fmt.Errorf("sync is off: set [sync] dir in config.toml, or -sync")
	}
	abs, err := filepath.Abs(*syncDir)
	if err != nil {
		return err
	}
	*syncDir = abs
	home, _ := os.UserHomeDir()
	if *hint {
		k, err := update.LoadOrCreateKey(*keyPath)
		if err != nil {
			return err
		}
		return printHint(out, *syncDir, home, *arch, k.Public().(ed25519.PublicKey))
	}
	if *version == "" || *builtAt == "" || fs.NArg() == 0 {
		return fmt.Errorf("usage: tockpub -version V -built-at RFC3339 [-sync DIR] [-key FILE] linux-ARCH...")
	}
	t, err := time.Parse(time.RFC3339, *builtAt)
	if err != nil {
		return fmt.Errorf("-built-at: %w", err)
	}
	key, err := filepath.Abs(*keyPath)
	if err != nil {
		return err
	}
	if inside(*syncDir, key) {
		return fmt.Errorf("key %s is inside the sync folder %s; the private key must not sync", key, *syncDir)
	}
	k, err := update.LoadOrCreateKey(key)
	if err != nil {
		return err
	}
	bin := filepath.Join(*syncDir, update.BinDir)
	var names []string
	for _, dir := range fs.Args() {
		arch, ok := strings.CutPrefix(filepath.Base(dir), "linux-")
		if !ok {
			return fmt.Errorf("%s: want a folder named linux-ARCH", dir)
		}
		if err := update.Publish(bin, arch, dir, *version, t, k); err != nil {
			return err
		}
		names = append(names, update.ArchDir(arch))
	}
	fmt.Fprintf(out, "published %s for %s in %s\n", *version, strings.Join(names, ", "), bin)
	return printHint(out, *syncDir, home, "amd64", k.Public().(ed25519.PublicKey))
}

// inside reports whether path is dir or below it.
func inside(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// printHint prints what a Linux machine needs once: the key's fingerprint,
// and a command that checks the build against sums it carries itself.
// Copy it from this terminal, not through the sync folder.
func printHint(out io.Writer, syncDir, home, arch string, pub ed25519.PublicKey) error {
	fp := update.Fingerprint(pub)
	fmt.Fprintf(out, "Update key fingerprint: %s\n", fp)
	fmt.Fprintf(out, "On a Linux machine with tock: tock update trust %s, and [sync] update = true in config.toml.\n", fp)
	cmd, err := update.Bootstrap(syncDir, home, arch, pub)
	if err != nil {
		fmt.Fprintf(out, "(no checked %s build to print an install command for: %v)\n", update.ArchDir(arch), err)
		return nil
	}
	fmt.Fprintf(out, "On a Linux machine without tock (or one from before updates), run once:\n\n  %s\n\n"+
		"It installs nothing unless both files match the sums in the command. Then set [sync] update = true there.\n"+
		"(This is for %s; make bootstrap-hint ARCH=arm64 prints the one for an ARM machine.)\n", cmd, update.ArchDir(arch))
	return nil
}

func dataDir() string {
	if d := os.Getenv("TOCK_DATA_DIR"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "share", "tock")
}

func configDir() string {
	if d := os.Getenv("TOCK_CONFIG_DIR"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "tock")
}
