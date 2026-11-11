package main

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"tock/internal/config"
	"tock/internal/update"
)

// cmdUpdate manages the key a Linux machine trusts for the builds the Mac
// publishes to the sync folder: tock update shows it, tock
// update trust <fingerprint> pins the key in the sync folder if it has that
// fingerprint. The fingerprint comes from the Mac's terminal (make
// bootstrap-hint), never from the sync folder.
func cmdUpdate(env Env, args []string) error {
	pinned := filepath.Join(env.ConfigDir, update.PubFile)
	fs := newFlags("update")
	syncFlag := fs.String("sync", "", "the sync folder (default: [sync] dir in config.toml)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	args = fs.Args()
	if len(args) == 2 { // flags after the subcommand: tock update trust FP --sync DIR
		if err := parseFlags(fs, args[2:]); err != nil {
			return err
		}
	}
	cfg, err := config.Load(env.configPath())
	if err != nil && *syncFlag == "" {
		return err
	}
	syncDir := cfg.Sync.Dir
	if *syncFlag != "" {
		syncDir = *syncFlag
	}
	switch {
	case len(args) == 0:
		b, err := os.ReadFile(pinned)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(env.Stdout, "no trusted key: on the Mac, make bootstrap-hint prints the fingerprint for tock update trust")
		} else if err != nil {
			return fmt.Errorf("update: %w", err)
		} else if pub, err := update.ParsePub(b); err != nil {
			return fmt.Errorf("update: %s: %w", pinned, err)
		} else {
			fmt.Fprintf(env.Stdout, "trusted key %s\n", update.Fingerprint(pub))
		}
		if cfg.Sync.Update {
			fmt.Fprintln(env.Stdout, "updates: on")
		} else {
			fmt.Fprintln(env.Stdout, "updates: off ([sync] update = true turns them on)")
		}
		return nil
	case len(args) == 2 && args[0] == "trust":
		if syncDir == "" {
			return errors.New("update: sync is off; set [sync] dir in config.toml, or pass --sync DIR")
		}
		offered := filepath.Join(syncDir, update.BinDir, update.PubFile)
		pub, err := update.Trust(pinned, offered, args[1])
		if errors.Is(err, update.ErrFingerprint) {
			return fmt.Errorf("update: the key in %s does not have fingerprint %s; nothing trusted. Check the fingerprint on the Mac (make bootstrap-hint)", offered, args[1])
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "trusted key %s\n", update.Fingerprint(ed25519.PublicKey(pub)))
		if !cfg.Sync.Update {
			fmt.Fprintln(env.Stdout, "Set [sync] update = true in config.toml for trackd to install the Mac's builds.")
		}
		return nil
	}
	return usagef("usage: tock update | tock update [--sync DIR] trust <fingerprint>")
}
