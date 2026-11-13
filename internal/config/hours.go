package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"tock/internal/stats"
)

type hoursTOML struct {
	Days  *[]string `toml:"days"`
	Start *string   `toml:"start"`
	End   *string   `toml:"end"`
	// OverGrace and OverRepeat tune the overworking notice (minutes); alone
	// they do not turn the schedule on.
	OverGrace  *int `toml:"over_grace"`
	OverRepeat *int `toml:"over_repeat"`
}

// dayNames are the [hours] days names, indexed by time.Weekday.
var dayNames = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// decodeHours reads the [hours] section. Any key turns the schedule on;
// start and end are then required, and days default to mon to fri.
func decodeHours(f hoursTOML, h *stats.Hours) error {
	if f.Days == nil && f.Start == nil && f.End == nil {
		return nil
	}
	if f.Start == nil || f.End == nil {
		return errors.New(`[hours] needs both start and end, like "09:00"`)
	}
	h.Set = true
	for _, k := range []struct {
		key string
		v   string
		dst *int
	}{{"start", *f.Start, &h.StartMin}, {"end", *f.End, &h.EndMin}} {
		at, ok := parseClock(k.v)
		if !ok || at < 0 {
			return fmt.Errorf(`[hours] %s = %q, want "HH:MM"`, k.key, k.v)
		}
		*k.dst = at
	}
	if h.StartMin >= h.EndMin {
		return fmt.Errorf("[hours] start %s is not before end %s", *f.Start, *f.End)
	}
	days := []string{"mon", "tue", "wed", "thu", "fri"}
	if f.Days != nil {
		days = *f.Days
		if len(days) == 0 {
			return errors.New("[hours] days is empty, want at least one of mon … sun")
		}
	}
	for _, d := range days {
		i := slices.Index(dayNames[:], strings.ToLower(strings.TrimSpace(d)))
		switch {
		case i < 0:
			return fmt.Errorf("[hours] days has %q, want mon, tue, wed, thu, fri, sat or sun", d)
		case h.Days[i]:
			return fmt.Errorf("[hours] days has %q twice", d)
		}
		h.Days[i] = true
	}
	return nil
}
