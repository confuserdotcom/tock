package config

import "fmt"

// Recommend is the [recommend] section: the focus suggestions and the
// adaptive break reminder tock learns from your history.
type Recommend struct {
	// Enabled turns recommendations on at all.
	Enabled bool
	// MaxPerDay is how many focus suggestions trackd sends a day; 0 = none.
	MaxPerDay int
	// AdaptiveBreak makes the break reminder follow the learned
	// sustained-focus length instead of [focus] break_after.
	AdaptiveBreak bool
	// MinDays is how many active days in the last 28 the profile needs
	// before it suggests anything.
	MinDays int
}

// DefaultRecommend is the [recommend] section used when the file does not set it.
func DefaultRecommend() Recommend {
	return Recommend{Enabled: true, MaxPerDay: 2, AdaptiveBreak: true, MinDays: 7}
}

type recommendTOML struct {
	Enabled       *bool `toml:"enabled"`
	MaxPerDay     *int  `toml:"max_per_day"`
	AdaptiveBreak *bool `toml:"adaptive_break"`
	MinDays       *int  `toml:"min_days"`
}

// decodeRecommend applies the keys present in f to c.
func decodeRecommend(f recommendTOML, c *Recommend) error {
	if f.Enabled != nil {
		c.Enabled = *f.Enabled
	}
	if f.AdaptiveBreak != nil {
		c.AdaptiveBreak = *f.AdaptiveBreak
	}
	if v := f.MaxPerDay; v != nil {
		if *v < 0 || *v > 10 {
			return fmt.Errorf("[recommend] max_per_day = %d, want 0 to 10", *v)
		}
		c.MaxPerDay = *v
	}
	if v := f.MinDays; v != nil {
		if *v < 3 || *v > 28 {
			return fmt.Errorf("[recommend] min_days = %d, want 3 to 28", *v)
		}
		c.MinDays = *v
	}
	return nil
}
