package config

import "fmt"

// DefaultPlan is the [plan] section used when the file does not set it:
// plan and calendar notices on, the up-next one 5 minutes ahead.
func DefaultPlan() Plan {
	return Plan{Notify: true, NotifyBeforeMS: 5 * 60_000}
}

// decodePlanNotices applies the notice keys of [plan] present in f to c.
func decodePlanNotices(f planTOML, c *Plan) error {
	if f.Notify != nil {
		c.Notify = *f.Notify
	}
	if f.NotifyBefore != nil {
		if *f.NotifyBefore < 0 || *f.NotifyBefore > 60 {
			return fmt.Errorf("[plan] notify_before = %d, want 0 to 60 minutes", *f.NotifyBefore)
		}
		c.NotifyBeforeMS = int64(*f.NotifyBefore) * 60_000
	}
	return nil
}
