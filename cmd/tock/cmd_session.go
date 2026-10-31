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
	s, err := a.StartSession(n)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "session started: %d min, ends %s\n", n,
		time.UnixMilli(s.PlannedMS).In(env.Now().Location()).Format("15:04"))
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

// sessionRate is `session rate [ID|last] N [note words...]`. The first
// argument is the id when it is `last`, or when it is all digits and the
// second argument is also all digits (`rate 42 4 words`); otherwise the id is
// `last` and the first argument is the rating (`rate 4 good run`). Without
// note words the stored note is kept; `session note` clears it.
func sessionRate(env Env, args []string) error {
	ref := "last"
	switch {
	case len(args) > 0 && args[0] == "last":
		args = args[1:]
	case len(args) > 1 && allDigits(args[0]) && allDigits(args[1]):
		ref, args = args[0], args[1:]
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

// sessionNote is `session note [ID|last] text...`; the first argument is the
// id when it is `last` or all digits. No text clears the note.
func sessionNote(env Env, args []string) error {
	ref := "last"
	if len(args) > 0 && (args[0] == "last" || allDigits(args[0])) {
		ref, args = args[0], args[1:]
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
