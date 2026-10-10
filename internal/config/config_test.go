package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/stats"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), File)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingIsDefault(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), File))
	if err != nil || c != Default() || c.Score != stats.DefaultWeights {
		t.Fatalf("Load(missing) = %+v, %v", c, err)
	}
}

func TestLoadValid(t *testing.T) {
	c, err := Load(write(t, "[score]\ndepth = 0.6\nblocks = 0.2\ncalm = 0.2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Score != (stats.Weights{Depth: 0.6, Blocks: 0.2, Calm: 0.2}) {
		t.Fatalf("weights = %+v", c.Score)
	}
}

func TestLoadPartialKeepsDefaults(t *testing.T) {
	// depth 0.5 is the default; moving 0.1 from blocks to calm needs both keys.
	c, err := Load(write(t, "[score]\nblocks = 0.2\ncalm = 0.3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Score != (stats.Weights{Depth: 0.5, Blocks: 0.2, Calm: 0.3}) {
		t.Fatalf("weights = %+v", c.Score)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	if c, err := Load(write(t, "")); err != nil || c != Default() {
		t.Fatalf("Load(empty) = %+v, %v", c, err)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct{ body, want string }{
		{"[score]\ndepht = 0.5\n", "config.toml: unknown key score.depht"},
		{"[sessions]\nlength = 25\n", "config.toml: unknown key sessions"},
		{"[score]\ndepth = 0.6\n", "config.toml: [score] weights sum to 1.10, want 1"},
		{"[score]\ndepth = 1.5\nblocks = -0.3\ncalm = -0.2\n", "config.toml: [score] depth = 1.5, want 0 to 1"},
		{"[score\n", "config.toml: "},
	}
	for _, tt := range tests {
		c, err := Load(write(t, tt.body))
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("Load(%q) error = %v, want prefix %q", tt.body, err, tt.want)
		}
		if c != Default() {
			t.Errorf("Load(%q) config = %+v, want Default()", tt.body, c)
		}
	}
}
