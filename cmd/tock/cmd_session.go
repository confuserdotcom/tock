package main

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/app"
	"tock/internal/config"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// cmdSession shows, starts or stops the focus session, and lists, rates and
// notes finished ones.
func cmdSession(env Env, args []string) error {
	if len(args) == 0 {
		return sessionShow(env)
	}
	switch args[0] {
	case "start":
		return sessionStart(env, args[1:])
	case "stop":
		if len(args) > 1 {
			return usagef("session stop takes no arguments")
		}
		return sessionStop(env)
	case "list":
		if len(args) > 1 {
			return usagef("session list takes no arguments")
		}
		return sessionList(env)
	case "rate":
		return sessionRate(env, args[1:])
	case "note":
		return sessionNote(env, args[1:])
	case "refile":
		return sessionRefile(env, args[1:])
	case "snooze":
		return sessionSnooze(env, args[1:])
	case "weights":
		if len(args) > 1 {
			return usagef("session weights takes no arguments")
		}
		return sessionWeights(env)
	}
	return usagef("session: unknown subcommand %q", args[0])
}

func sessionShow(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	f, err := a.Focus(env.Now())
	if err != nil {
		return err
	}
	if f.Open == nil {
		fmt.Fprintln(env.Stdout, "no session")
		return nil
	}
	fmt.Fprintln(env.Stdout, "session "+report.SessionStatus(*f.Open, f.Live, f.NowMS))
	return nil
}

func sessionStart(env Env, args []string) error {
	paper, goal := false, ""
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--paper":
			paper = true
		case a == "--goal":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return usagef("session start: --goal needs what the session is for")
			}
			i++
			goal = args[i]
		case strings.HasPrefix(a, "--goal="):
			goal = strings.TrimPrefix(a, "--goal=")
		case isHelpFlag(a):
			return errHelp
		case strings.HasPrefix(a, "-"):
			return usagef("session start: unknown flag %q (--paper: idle time is paper work; --goal \"text\": what it is for)", a)
		default:
			rest = append(rest, a)
		}
	}
	args = rest
	if len(args) > 1 {
		return usagef("session start takes one length in minutes")
	}
	n := 0
	if len(args) == 1 {
		v, err := strconv.Atoi(args[0])
		if err != nil || v < 1 || v > config.MaxSessionMin {
			return usagef("session start: length %q, want 1 to %d minutes", args[0], config.MaxSessionMin)
		}
		n = v
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if n == 0 {
		n = a.Config().Focus.Presets[0]
	}
	s, err := a.StartSessionGoal(n, goal)
	if err != nil {
		return err
	}
	tail := ""
	if paper {
		if err := a.SetPaper(s.ID); err != nil {
			return err
		}
		tail = ", on paper"
	}
	if s.Goal != "" {
		tail += " · " + s.Goal
	}
	fmt.Fprintf(env.Stdout, "session started: %d min, ends %s%s\n", n,
		time.UnixMilli(s.PlannedMS).In(env.Now().Location()).Format("15:04"), tail)
	if h, err := a.Health(); err == nil && !h.TrackdUp {
		fmt.Fprintln(env.Stderr, "warning: trackd not running, no alerts")
	}
	return nil
}

func sessionStop(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	s, err := a.StopSession()
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, "stopped: "+report.SessionLine(s, env.Now().Location(), false))
	return nil
}

// sessionSnooze snoozes the running session's distraction notices for MIN
// minutes (default [focus] snooze), or with "off" resumes them.
func sessionSnooze(env Env, args []string) error {
	if len(args) > 1 {
		return usagef("session snooze takes minutes or off")
	}
	mins, off := 0, false
	if len(args) == 1 {
		if args[0] == "off" {
			off = true
		} else {
			n, err := strconv.Atoi(args[0])
			switch {
			case err != nil:
				return usagef("session snooze: %q, want minutes or off", args[0])
			case n < 1 || n > config.MaxSnoozeMin:
				return usagef("session snooze: %d min is out of range, want 1 to %d", n, config.MaxSnoozeMin)
			}
			mins = n
		}
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if off {
		was, err := a.ResumeDistract()
		if err != nil {
			return err
		}
		if was {
			fmt.Fprintln(env.Stdout, "distraction notices back on")
		} else {
			fmt.Fprintln(env.Stdout, "distraction notices were not snoozed")
		}
		return nil
	}
	d := time.Duration(mins) * time.Minute
	if mins == 0 {
		d = time.Duration(a.Config().Focus.SnoozeMS) * time.Millisecond
	}
	until, err := a.SnoozeDistract(0, d)
	if errors.Is(err, store.ErrNoSession) {
		return errors.New("session snooze: no session running (tock session start 50)")
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "distraction notices snoozed for %d min, until %s\n", int(d.Minutes()), until.Format("15:04"))
	return nil
}

const noSessionYet = "no finished session yet (tock session start 50)"

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// splitSessionRef takes the session a command is about off the front of
// args. Only unambiguous forms name one: `#42`, `--id 42` and `--id=42`, and
// the word `last` when allowLast is set. A bare number is never an id, so a
// rating or the first word of a note cannot land on another session. With no
// such form the session is "last".
func splitSessionRef(what string, args []string, allowLast bool) (ref string, rest []string, err error) {
	if len(args) == 0 {
		return "last", args, nil
	}
	first := args[0]
	switch {
	case allowLast && first == "last":
		return "last", args[1:], nil
	case strings.HasPrefix(first, "#") && allDigits(first[1:]):
		return first[1:], args[1:], nil
	case first == "--id":
		if len(args) < 2 || !allDigits(args[1]) {
			return "", nil, usagef("%s: --id needs a session number", what)
		}
		return args[1], args[2:], nil
	case strings.HasPrefix(first, "--id="):
		if !allDigits(first[len("--id="):]) {
			return "", nil, usagef("%s: --id needs a session number", what)
		}
		return first[len("--id="):], args[1:], nil
	}
	return "last", args, nil
}

// sessionTarget resolves "last" or a numeric id to a finished-or-not session.
// Errors read "no finished session yet ..." or "no session N ...".
func sessionTarget(a *app.App, ref string) (focus.Session, error) {
	if ref == "last" {
		s, err := a.LastSession()
		if err != nil {
			return focus.Session{}, err
		}
		if s == nil {
			return focus.Session{}, errors.New(noSessionYet)
		}
		return *s, nil
	}
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		return focus.Session{}, fmt.Errorf("no session %s (tock session list shows ids)", ref)
	}
	s, err := a.SessionByID(id)
	if errors.Is(err, store.ErrSessionGone) {
		return focus.Session{}, fmt.Errorf("no session %d (tock session list shows ids)", id)
	}
	return s, err
}

// checkNote cleans the words of a note and refuses one over the limit, so a
// CLI user is told instead of silently truncated. what is the command name.
func checkNote(what string, words []string) (string, error) {
	note, n := report.CleanNote(strings.Join(words, " "))
	if n > report.MaxNoteRunes {
		return "", fmt.Errorf("%s: note is %d characters, max %d", what, n, report.MaxNoteRunes)
	}
	return note, nil
}

// sessionRate is `session rate [#ID|--id ID|last] N [note words...]`. A
// leading number is the rating, never an id: `rate 4 2 hours deep` rates the
// last session 4 stars with the note "2 hours deep". Without note words the
// stored note is kept; `session note` clears it.
func sessionRate(env Env, args []string) error {
	ref, args, err := splitSessionRef("session rate", args, true)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return usagef("session rate: give a rating 1 to 5")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 || n > 5 {
		return usagef("session rate: rating %q, want 1 to 5", args[0])
	}
	note, err := checkNote("session rate", args[1:])
	if err != nil {
		return err
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	s, err := sessionTarget(a, ref)
	if err != nil {
		return fmt.Errorf("session rate: %w", err)
	}
	var np *string
	if note != "" {
		np = &note
	}
	if s, _, err = a.RateSession(s.ID, n, np); err != nil {
		return fmt.Errorf("session rate: %w", err)
	}
	w, _, _, err := a.Weights()
	if err != nil {
		return fmt.Errorf("session rate: %w", err)
	}
	out := fmt.Sprintf("rated session %d (%s, %s, q%d) ★%d", s.ID,
		time.UnixMilli(s.StartMS).In(env.Now().Location()).Format("Mon 15:04"),
		report.Dur(s.EndMS-s.StartMS), focus.Quality(s, w).Score, n)
	if np != nil {
		out += " · note saved"
	}
	fmt.Fprintln(env.Stdout, out)
	return nil
}

// sessionNote is `session note [#ID|--id ID] text...`. The words are the
// note, so a leading number or the word last is text; only #ID and --id name
// another session than the last. No text clears the note.
func sessionNote(env Env, args []string) error {
	ref, args, err := splitSessionRef("session note", args, false)
	if err != nil {
		return err
	}
	note, err := checkNote("session note", args)
	if err != nil {
		return err
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	s, err := sessionTarget(a, ref)
	if err != nil {
		return fmt.Errorf("session note: %w", err)
	}
	if s, err = a.SetNote(s.ID, note); err != nil {
		return fmt.Errorf("session note: %w", err)
	}
	if s.Note == "" {
		fmt.Fprintf(env.Stdout, "cleared the note on session %d\n", s.ID)
		return nil
	}
	fmt.Fprintf(env.Stdout, "noted session %d: %s\n", s.ID, s.Note)
	return nil
}

// sessionRefile is `session refile [#ID|--id ID|last] PROJECT`; the rest
// joined by a space is the project (names may hold spaces or digits).
func sessionRefile(env Env, args []string) error {
	ref, args, err := splitSessionRef("session refile", args, true)
	if err != nil {
		return err
	}
	project := strings.TrimSpace(strings.Join(args, " "))
	if project == "" {
		return usagef("session refile: PROJECT is required")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	s, err := sessionTarget(a, ref)
	if err != nil {
		return fmt.Errorf("session refile: %w", err)
	}
	s, name, err := a.RefileSession(s.ID, project)
	if err != nil {
		return fmt.Errorf("session refile: %w", err)
	}
	fmt.Fprintf(env.Stdout, "refiled session %d (%s) to %s · tock undo reverts it\n", s.ID, report.Dur(s.EndMS-s.StartMS), name)
	return nil
}

// sessionList prints this week's finished sessions, newest first.
func sessionList(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	f, err := a.Focus(env.Now())
	if err != nil {
		return err
	}
	rows := append(append([]focus.Session(nil), f.Today...), f.Week...)
	if len(rows) == 0 {
		fmt.Fprintln(env.Stdout, "no sessions this week")
		return nil
	}
	w, _, _, err := a.Weights()
	if err != nil {
		return err
	}
	width := len("id")
	for _, s := range rows {
		width = max(width, len(strconv.FormatInt(s.ID, 10)))
	}
	loc := env.Now().Location()
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%*s\twhen\tkind\tlen\tdeep\tq\t★\tint\tnote\n", width, "id")
	for _, s := range rows {
		star, ints := "-", "-"
		if s.Rating > 0 {
			star = strconv.Itoa(s.Rating)
		}
		if s.Kind == focus.KindManual {
			ints = strconv.Itoa(s.Interruptions)
		}
		note, _ := report.CleanNote(s.Note)
		if w := report.WakeText(s); w != "" {
			note = strings.TrimSuffix(w+" · "+note, " · ")
		}
		fmt.Fprintf(tw, "%*d\t%s\t%s\t%s\t%d%%\t%d\t%s\t%s\t%s\n", width, s.ID,
			time.UnixMilli(s.StartMS).In(loc).Format("Mon 15:04"), s.Kind, report.Dur(s.EndMS-s.StartMS),
			report.DeepPct(s, f.NowMS), focus.Quality(s, w).Score, star, ints, note)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		l = strings.TrimRight(l, " ")
		if ansi.StringWidth(l) > 80 {
			l = report.Cut(l, 80)
		}
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}

// sessionWeights prints the default and the current quality weights.
func sessionWeights(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	cur, n, fitted, err := a.Weights()
	if err != nil {
		return err
	}
	def := focus.DefaultQualityWeights()
	fmt.Fprintln(env.Stdout, "component  default  current")
	for _, c := range []struct {
		name     string
		def, cur float64
	}{
		{"share", def.Share, cur.Share}, {"longest", def.Longest, cur.Longest},
		{"calm", def.Calm, cur.Calm}, {"steady", def.Steady, cur.Steady},
	} {
		fmt.Fprintf(env.Stdout, "%-9s  %7.2f  %7.2f\n", c.name, c.def, c.cur)
	}
	if fitted {
		fmt.Fprintf(env.Stdout, "based on %d rated sessions\n", n)
	} else {
		fmt.Fprintf(env.Stdout, "default (%d/%d rated)\n", n, focus.MinRated)
	}
	return nil
}
