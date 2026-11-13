package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Slack is the [slack] section: the status and do-not-disturb tock sets in
// Slack while a focus session runs.
type Slack struct {
	Enabled bool
	Status  string // status text
	Emoji   string // ":name:"
	DND     bool   // snooze notifications for the session
}

// DefaultSlack is the [slack] section used when the file does not set it.
func DefaultSlack() Slack {
	return Slack{Status: "In a focus session", Emoji: ":dart:", DND: true}
}

type slackTOML struct {
	Enabled *bool   `toml:"enabled"`
	Status  *string `toml:"status"`
	Emoji   *string `toml:"emoji"`
	DND     *bool   `toml:"dnd"`
}

// emojiRE is a Slack emoji code.
var emojiRE = regexp.MustCompile(`^:[a-z0-9_+'-]+:$`)

// decodeSlack applies the keys present in f to c and validates them.
func decodeSlack(f slackTOML, c *Slack) error {
	if f.Enabled != nil {
		c.Enabled = *f.Enabled
	}
	if f.Status != nil {
		s := strings.TrimSpace(*f.Status)
		if s == "" || len([]rune(s)) > 100 {
			return fmt.Errorf("[slack] status = %q, want 1 to 100 characters", *f.Status)
		}
		c.Status = s
	}
	if f.Emoji != nil {
		if !emojiRE.MatchString(*f.Emoji) {
			return fmt.Errorf(`[slack] emoji = %q, want a code such as ":dart:"`, *f.Emoji)
		}
		c.Emoji = *f.Emoji
	}
	if f.DND != nil {
		c.DND = *f.DND
	}
	return nil
}
