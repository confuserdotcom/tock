// Package slack sets the Slack status and do-not-disturb for a focus
// session. Only cmd/tock imports it; trackd runs tock for it. Errors carry
// the method and Slack's error code only, never the token.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultURL is the Web API base.
const DefaultURL = "https://slack.com/api/"

// Timeout bounds one request.
const Timeout = 15 * time.Second

// Client calls the Slack Web API with a user token.
type Client struct {
	Token string
	URL   string // base, ending in "/"
	HTTP  *http.Client
}

// New returns a client for token.
func New(token string) *Client {
	return &Client{Token: token, URL: DefaultURL, HTTP: &http.Client{Timeout: Timeout}}
}

// Profile is the status part of a Slack profile.
type Profile struct {
	StatusText       string `json:"status_text"`
	StatusEmoji      string `json:"status_emoji"`
	StatusExpiration int64  `json:"status_expiration"` // unix seconds; 0 = never
}

// call posts body as JSON (or a GET when body is nil) and decodes the reply
// into out. A reply with ok false is an error naming Slack's code.
func (c *Client) call(ctx context.Context, method string, body any, out any) error {
	var req *http.Request
	var err error
	if body == nil {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, c.URL+method, nil)
	} else {
		b, merr := json.Marshal(body)
		if merr != nil {
			return merr
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.URL+method, bytes.NewReader(b))
		if req != nil {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
	}
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("slack %s: request failed", method)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("slack %s: read: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack %s: HTTP %d", method, resp.StatusCode)
	}
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("slack %s: bad reply", method)
	}
	if !env.OK {
		return fmt.Errorf("slack %s: %s", method, env.Error)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Status returns the current status.
func (c *Client) Status(ctx context.Context) (Profile, error) {
	var r struct {
		Profile Profile `json:"profile"`
	}
	err := c.call(ctx, "users.profile.get", nil, &r)
	return r.Profile, err
}

// SetStatus sets the status; an empty Profile clears it.
func (c *Client) SetStatus(ctx context.Context, p Profile) error {
	return c.call(ctx, "users.profile.set", map[string]any{"profile": p}, nil)
}

// Snooze turns notifications off for minutes minutes.
func (c *Client) Snooze(ctx context.Context, minutes int) error {
	return c.call(ctx, "dnd.setSnooze", map[string]any{"num_minutes": minutes}, nil)
}

// EndSnooze turns notifications back on.
func (c *Client) EndSnooze(ctx context.Context) error {
	return c.call(ctx, "dnd.endSnooze", map[string]any{}, nil)
}

// Start sets the session status until untilMS and, with dnd, snoozes
// notifications as long.
func (c *Client) Start(ctx context.Context, text, emoji string, dnd bool, nowMS, untilMS int64) error {
	if err := c.SetStatus(ctx, Profile{StatusText: text, StatusEmoji: emoji, StatusExpiration: untilMS / 1000}); err != nil {
		return err
	}
	if !dnd {
		return nil
	}
	return c.Snooze(ctx, int(max((untilMS-nowMS+59_999)/60_000, 1)))
}

// Stop clears the status when it is still the one Start set (the user may
// have set another meanwhile), and with dnd ends the snooze.
func (c *Client) Stop(ctx context.Context, text, emoji string, dnd bool) error {
	p, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if p.StatusText == text && p.StatusEmoji == emoji {
		if err := c.SetStatus(ctx, Profile{}); err != nil {
			return err
		}
	}
	if !dnd {
		return nil
	}
	if err := c.EndSnooze(ctx); err != nil && err.Error() != "slack dnd.endSnooze: snooze_not_active" {
		return err
	}
	return nil
}
