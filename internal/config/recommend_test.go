package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadRecommend(t *testing.T) {
	c, err := Load(write(t, ""))
	want := Recommend{Enabled: true, MaxPerDay: 2, AdaptiveBreak: true, MinDays: 7}
	if err != nil || c.Recommend != want {
		t.Fatalf("empty file: %+v, err = %v, want %+v", c.Recommend, err, want)
	}
	m, err := Load(filepath.Join(t.TempDir(), File))
	if err != nil || !reflect.DeepEqual(m, Default()) {
		t.Fatalf("missing file: got %+v, %v, want Default()", m, err)
	}
	if DefaultRecommend() != want {
		t.Fatalf("DefaultRecommend() = %+v, want %+v", DefaultRecommend(), want)
	}
	for _, v := range []string{"0", "10"} {
		if _, err := Load(write(t, "[recommend]\nmax_per_day = "+v+"\n")); err != nil {
			t.Errorf("max_per_day = %s: %v", v, err)
		}
	}
	for _, v := range []string{"11", "-1"} {
		_, err := Load(write(t, "[recommend]\nmax_per_day = "+v+"\n"))
		if err == nil || !strings.Contains(err.Error(), "config.toml: [recommend] max_per_day = "+v+", want 0 to 10") {
			t.Errorf("max_per_day = %s: err = %v", v, err)
		}
	}
	for _, v := range []string{"3", "28"} {
		if _, err := Load(write(t, "[recommend]\nmin_days = "+v+"\n")); err != nil {
			t.Errorf("min_days = %s: %v", v, err)
		}
	}
	for _, v := range []string{"2", "29"} {
		_, err := Load(write(t, "[recommend]\nmin_days = "+v+"\n"))
		if err == nil || !strings.Contains(err.Error(), "config.toml: [recommend] min_days = "+v+", want 3 to 28") {
			t.Errorf("min_days = %s: err = %v", v, err)
		}
	}
	_, err = Load(write(t, "[recommend]\nbogus = 1\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key recommend.bogus") {
		t.Fatalf("unknown key: err = %v", err)
	}
	c, err = Load(write(t, "[recommend]\nenabled = false\nadaptive_break = false\n"))
	if err != nil || c.Recommend.Enabled || c.Recommend.AdaptiveBreak || c.Recommend.MaxPerDay != 2 {
		t.Fatalf("enabled/adaptive_break false: %+v, err = %v", c.Recommend, err)
	}
}
