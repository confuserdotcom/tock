// Package config reads the optional ~/.config/tock/config.toml.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"tock/internal/stats"
)

// File is the config file name in tock's config directory.
const File = "config.toml"

// Config is tock's settings.
type Config struct{ Score stats.Weights }

// Default is the configuration used when there is no file.
func Default() Config { return Config{Score: stats.DefaultWeights} }

type scoreTOML struct {
	Depth  *float64 `toml:"depth"`
	Blocks *float64 `toml:"blocks"`
	Calm   *float64 `toml:"calm"`
}

type fileTOML struct {
	Score scoreTOML `toml:"score"`
}

// Load reads path. A missing file is Default() and no error. Any other
// problem returns Default() and an error naming the file.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	name := filepath.Base(path)
	if err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	var f fileTOML
	md, err := toml.Decode(string(b), &f)
	if err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if un := md.Undecoded(); len(un) > 0 {
		keys := make([]string, len(un))
		for i, k := range un {
			keys[i] = k.String()
		}
		return Default(), fmt.Errorf("%s: unknown key %s", name, strings.Join(keys, ", "))
	}
	for _, w := range []struct {
		key string
		v   *float64
		dst *float64
	}{{"depth", f.Score.Depth, &c.Score.Depth}, {"blocks", f.Score.Blocks, &c.Score.Blocks}, {"calm", f.Score.Calm, &c.Score.Calm}} {
		if w.v == nil {
			continue
		}
		if *w.v < 0 || *w.v > 1 || math.IsNaN(*w.v) {
			return Default(), fmt.Errorf("%s: [score] %s = %g, want 0 to 1", name, w.key, *w.v)
		}
		*w.dst = *w.v
	}
	if sum := c.Score.Depth + c.Score.Blocks + c.Score.Calm; math.Abs(sum-1) > 0.001 {
		return Default(), fmt.Errorf("%s: [score] weights sum to %.2f, want 1", name, sum)
	}
	return c, nil
}
