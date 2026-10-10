package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tock/internal/config"
	"tock/internal/slack"
)

// slackTokenFile holds the Slack user token, owner-only.
const slackTokenFile = "slack.token"

func (e Env) slackTokenPath() string { return filepath.Join(e.ConfigDir, slackTokenFile) }

// slackToken is TOCK_SLACK_TOKEN, else the token file, else "".
func (e Env) slackToken() string {
	if e.SlackToken != "" {
		return e.SlackToken
	}
	b, err := os.ReadFile(e.slackTokenPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (e Env) slackClient(token string) *slack.Client {
	if e.NewSlack != nil {
		return e.NewSlack(token)
	}
	return slack.New(token)
}

// cmdSlack sets the Slack status for focus sessions. trackd runs start and
// stop; login stores the token.
func cmdSlack(env Env, args []string) error {
	sub := "status"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	cfg, err := config.Load(filepath.Join(env.ConfigDir, config.File))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), slack.Timeout)
	defer cancel()
	switch sub {
	case "login":
		return slackLogin(env)
	case "logout":
		if err := os.Remove(env.slackTokenPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Fprintln(env.Stdout, "slack: token removed")
		return nil
	case "status":
		state := "off ([slack] enabled = true in config.toml)"
		if cfg.Slack.Enabled {
			state = "on"
		}
		fmt.Fprintf(env.Stdout, "slack: %s\n", state)
		tok := env.slackToken()
		if tok == "" {
			fmt.Fprintln(env.Stdout, "token: missing (tock slack login)")
			return nil
		}
		p, err := env.slackClient(tok).Status(ctx)
		if err != nil {
			return err
		}
		cur := "none"
		if p.StatusText != "" || p.StatusEmoji != "" {
			cur = strings.TrimSpace(p.StatusEmoji + " " + p.StatusText)
		}
		fmt.Fprintf(env.Stdout, "token: ok\ncurrent status: %s\n", cur)
		return nil
	case "start", "stop":
		fs := newFlags("slack " + sub)
		until := fs.Int64("until", 0, "unix ms the session ends")
		if err := parseFlags(fs, args); err != nil {
			return err
		}
		if !cfg.Slack.Enabled {
			return nil // turned off since trackd queued the hook
		}
		tok := env.slackToken()
		if tok == "" {
			return errors.New("slack: no token (tock slack login)")
		}
		c := env.slackClient(tok)
		s := cfg.Slack
		if sub == "stop" {
			return c.Stop(ctx, s.Status, s.Emoji, s.DND)
		}
		now := env.Now().UnixMilli()
		if *until <= now {
			return usagef("slack start: --until %d is not in the future", *until)
		}
		return c.Start(ctx, s.Status, s.Emoji, s.DND, now, *until)
	}
	return usagef("slack: unknown subcommand %q (login, logout, status, start, stop)", sub)
}

// slackLogin reads a user token (xoxp-…) from stdin and writes it 0600.
func slackLogin(env Env) error {
	fmt.Fprintln(env.Stderr, "Paste a Slack user token (xoxp-…) with users.profile:read, users.profile:write and dnd:write, then enter:")
	line, err := bufio.NewReader(env.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("slack login: no token read")
	}
	tok := strings.TrimSpace(line)
	if !strings.HasPrefix(tok, "xoxp-") || strings.ContainsAny(tok, " \t") {
		return errors.New("slack login: want a user token starting with xoxp-")
	}
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	tmp := env.slackTokenPath() + ".tmp" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmp, []byte(tok+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, env.slackTokenPath()); err != nil {
		os.Remove(tmp)
		return err
	}
	fmt.Fprintf(env.Stdout, "slack: token saved to %s\n", env.slackTokenPath())
	return nil
}
