package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/store"
)

const (
	// BarFreshMS is how old the menu bar app's heartbeat may be for trackd
	// to hand it notifications.
	BarFreshMS = 15_000
	// NoticeMaxAgeMS is how old a queued notice may be and still be shown.
	NoticeMaxAgeMS = 2 * 60_000
	// AwayQuickLabels is how many recent away labels an away notice offers.
	AwayQuickLabels = 3
)

// Notification access states the menu bar app reports.
const (
	NotifyGranted = "granted"
	NotifyDenied  = "denied"
	NotifyUnknown = "unknown"
)

// NoticeData is what a notice's buttons need: the finished session's
// length, id and top project, or the absence and the labels offered for it.
// Every key is optional so notices queued by an older trackd still decode.
type NoticeData struct {
	Minutes   int             `json:"minutes,omitempty"`
	Session   int64           `json:"session,omitempty"`    // done or focus: the session's id, 0 = none
	Project   string          `json:"project,omitempty"`    // done: its top project
	Refile    bool            `json:"refile,omitempty"`     // done: refiling to Project is offered
	SnoozeMin int             `json:"snooze_min,omitempty"` // focus: the minutes the Snooze button offers
	Category  string          `json:"category,omitempty"`   // focus: the category the notice offers to mark non-interrupting
	Rule      string          `json:"rule,omitempty"`       // focus: the rule ref that set Category, "" = none offered
	RuleFP    string          `json:"rule_fp,omitempty"`    // focus: that rule's Fingerprint() when the notice was queued
	Urge      bool            `json:"urge,omitempty"`       // focus: the notice is an urge-surfing prompt
	From      int64           `json:"from,omitempty"`
	To        int64           `json:"to,omitempty"`
	Labels    []classify.Want `json:"labels,omitempty"`
}

// Enqueue queues the notification ev for the menu bar app.
func (a *App) Enqueue(ev focus.Event) error {
	d := NoticeData{Minutes: ev.Minutes}
	if ev.Topic == focus.TopicDone {
		d.Session, d.Project, d.Refile = ev.Sess.ID, ev.Project, ev.Refile
	}
	if ev.Topic == focus.TopicFocus {
		d.Session, d.SnoozeMin = ev.Sess.ID, int(a.cfg.Focus.SnoozeMS/60_000)
		d.Category, d.Urge = ev.Category, ev.Urge
		if r, ok := a.rs.RuleByRef(ev.RuleRef); ok && ev.RuleRef != "" && strings.EqualFold(r.Category, ev.Category) {
			d.Rule, d.RuleFP = r.Ref(), r.Fingerprint()
		}
	}
	if ev.Topic == focus.TopicAway {
		d.From, d.To = ev.Away.From, ev.Away.To
		ls, err := a.RecentAwayLabels(AwayQuickLabels)
		if err != nil {
			return err
		}
		if b := ev.Before; b.Category != "" {
			ls = slices.DeleteFunc(ls, func(w classify.Want) bool { return w == b })
			ls = append([]classify.Want{b}, ls[:min(len(ls), AwayQuickLabels-1)]...)
		}
		d.Labels = ls
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = a.st.AddNotice(store.Notice{AtMS: a.nowMS(), Topic: ev.Topic, Title: ev.Title, Body: ev.Body, Data: string(b)})
	return err
}

// TakeNotices returns the notices to show now; see store.TakeNotices.
func (a *App) TakeNotices() ([]store.Notice, error) {
	now := a.nowMS()
	return a.st.TakeNotices(now, now-NoticeMaxAgeMS)
}

// Notice returns the notice with id and its decoded data.
func (a *App) Notice(id int64) (store.Notice, NoticeData, error) {
	n, err := a.st.Notice(id)
	if err != nil {
		return n, NoticeData{}, err
	}
	var d NoticeData
	if err := json.Unmarshal([]byte(n.Data), &d); err != nil {
		return n, NoticeData{}, fmt.Errorf("notice %d: %w", id, err)
	}
	return n, d, nil
}

// BarBeat records the menu bar app's heartbeat and notification access.
func (a *App) BarBeat(access string) error {
	now := a.nowMS()
	if err := a.st.SetState(store.KeyBarBeat, strconv.FormatInt(now, 10), now); err != nil {
		return err
	}
	return a.st.SetState(store.KeyBarNotify, access, now)
}

// BarState reports whether the menu bar app is running (its heartbeat is
// fresh) and its notification access.
func (a *App) BarState() (running bool, access string, err error) {
	h, err := ReadHealth(a.st, a.nowMS())
	return h.BarUp, h.BarNotify, err
}

// BarReady reports whether trackd should hand notifications to the menu
// bar app: it is running and may notify.
func (a *App) BarReady() bool {
	running, access, err := a.BarState()
	return err == nil && running && access == NotifyGranted
}
