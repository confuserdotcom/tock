package main

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/update"
)

func TestUpdateTrust(t *testing.T) {
	te := newTestEnv(t, testRules)
	syncDir := filepath.Join(t.TempDir(), "sync")
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[sync]\ndir = \""+syncDir+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := update.LoadOrCreateKey(filepath.Join(t.TempDir(), "k", "update.key"))
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	for _, n := range update.Names {
		os.WriteFile(filepath.Join(src, n), []byte(n), 0o755)
	}
	if err := update.Publish(filepath.Join(syncDir, update.BinDir), "amd64", src, "v2", time.Now(), key); err != nil {
		t.Fatal(err)
	}
	fp := update.Fingerprint(key.Public().(ed25519.PublicKey))
	pinned := filepath.Join(te.ConfigDir, update.PubFile)

	if _, out := te.run(t, "", "update"); !strings.Contains(out, "no trusted key") || !strings.Contains(out, "updates: off") {
		t.Fatalf("status before:\n%s", out)
	}
	wrong := strings.Repeat("0", 64)
	if code, _ := te.run(t, "", "update", "trust", wrong); code == 0 || !strings.Contains(te.errb.String(), "nothing trusted") {
		t.Fatalf("wrong fingerprint: code %d err %s", code, te.errb)
	}
	if _, err := os.Stat(pinned); !os.IsNotExist(err) {
		t.Fatal("pinned with a wrong fingerprint")
	}
	code, out := te.run(t, "", "update", "trust", fp)
	if code != 0 || !strings.Contains(out, "trusted key "+fp) || !strings.Contains(out, "[sync] update = true") {
		t.Fatalf("right fingerprint: code %d out %s err %s", code, out, te.errb)
	}
	if _, out := te.run(t, "", "update"); !strings.Contains(out, "trusted key "+fp) {
		t.Fatalf("status after:\n%s", out)
	}
}
