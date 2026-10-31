package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/store"
)

var (
	// ErrRuleChanged means the rule a notice named is not that rule any
	// more: it moved, was edited or recategorised since the notice.
	ErrRuleChanged = errors.New("rule changed")
	// ErrCategoryGone means the category a notice named no longer exists.
	ErrCategoryGone = errors.New("no such category")
)

// interruptValue is the interrupt line a mark writes: "false" marks, "" (no
// line) is the default and unmarks.
func interruptValue(on bool) string {
	if on {
		return ""
	}
	return "false"
}

// SetCategoryInterrupt marks the category name as one that never interrupts
// a session (on false) or removes the mark (on true), in rules.toml. The
// name must match exactly, ignoring case: a click on a stale notice never
// reaches a different category by prefix. It fails with ErrCategoryGone
// when the category no longer exists. Config, not labels: no undo entry.
func (a *App) SetCategoryInterrupt(name string, on bool) error {
	return a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		rs, err := classify.Parse(rules, learned)
		if err != nil {
			return nil, nil, err
		}
		if !rs.HasCategory(name) {
			return nil, nil, fmt.Errorf("app: category %q: %w", name, ErrCategoryGone)
		}
		text, err := classify.SetTableField(string(rules), classify.NameCategory, name, "interrupt", interruptValue(on))
		if err != nil {
			return nil, nil, err
		}
		return []byte(text), learned, nil
	})
}

// MarkRuleQuiet writes interrupt = false on the rule ref names, in the file
// it lives in. The files are read again and the rule is refused with
// ErrRuleChanged unless it still has the fingerprint (classify.Rule.Fingerprint)
// and category the notice recorded, so a rule that moved, was edited or was
// renumbered by a newly learned rule is never marked by mistake. Config, not
// labels: no undo entry.
func (a *App) MarkRuleQuiet(ref, fingerprint, category string) error {
	return a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		rs, err := classify.Parse(rules, learned)
		if err != nil {
			return nil, nil, err
		}
		r, ok := rs.RuleByRef(ref)
		if !ok || r.Fingerprint() != fingerprint || !strings.EqualFold(r.Category, category) {
			return nil, nil, fmt.Errorf("app: mark %s: %w", ref, ErrRuleChanged)
		}
		if r.File == classify.LearnedFile {
			out, err := classify.SetRuleField(r.File, learned, r.Index-1, "interrupt", "false")
			return rules, out, err
		}
		out, err := classify.SetRuleField(r.File, rules, r.Index-1, "interrupt", "false")
		return out, learned, err
	})
}

// SnoozeDistract holds the distraction notices of the running session back
// for d from now and returns when they resume, in the App clock's zone.
// session is the id of the session a notice was about; 0 means whatever
// runs. It fails with store.ErrNoSession when no session runs or the running
// one is not session, so a late click on an old notice snoozes nothing.
func (a *App) SnoozeDistract(session int64, d time.Duration) (time.Time, error) {
	open, err := a.st.OpenSession()
	if err != nil {
		return time.Time{}, err
	}
	if open == nil || (session > 0 && open.ID != session) {
		return time.Time{}, store.ErrNoSession
	}
	until := a.now().Add(d)
	if err := a.st.SetState(store.KeyFocusDistractSnooze, strconv.FormatInt(until.UnixMilli(), 10), a.nowMS()); err != nil {
		return time.Time{}, err
	}
	return until, nil
}

// ResumeDistract ends a distraction snooze now. It reports whether
// notices were snoozed.
func (a *App) ResumeDistract() (bool, error) {
	until, err := a.distractSnooze()
	if err != nil {
		return false, err
	}
	now := a.nowMS()
	if err := a.st.SetState(store.KeyFocusDistractSnooze, "0", now); err != nil {
		return false, err
	}
	return until > now, nil
}

// distractSnooze is when the distraction snooze ends (ms), 0 when absent.
func (a *App) distractSnooze() (int64, error) {
	m, err := a.st.States()
	if err != nil {
		return 0, err
	}
	until, _ := strconv.ParseInt(m[store.KeyFocusDistractSnooze], 10, 64)
	return until, nil
}

// AddUrge counts an answer to an urge-surfing notice on that session, given
// in when given is true and resisted otherwise, even after the session ended.
// It fails with store.ErrSessionGone when the session no longer exists.
func (a *App) AddUrge(session int64, given bool) error {
	return a.st.AddUrge(session, given)
}
