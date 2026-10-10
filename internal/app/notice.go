package app

import (
	"encoding/json"
	"fmt"
	"strconv"

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
// length, or the absence and the labels offered for it.
type NoticeData struct {
	Minutes int             `json:"minutes,omitempty"`
	From    int64           `json:"from,omitempty"`
	To      int64           `json:"to,omitempty"`
	Labels  []classify.Want `json:"labels,omitempty"`
}

// Enqueue queues the notification ev for the menu bar app.
func (a *App) Enqueue(ev focus.Event) error {
	d := NoticeData{Minutes: ev.Minutes}
	if ev.Topic == focus.TopicAway {
		d.From, d.To = ev.Away.From, ev.Away.To
		ls, err := a.RecentAwayLabels(AwayQuickLabels)
		if err != nil {
			return err
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
	m, err := a.st.States()
	if err != nil {
		return false, "", err
	}
	beat, _ := strconv.ParseInt(m[store.KeyBarBeat], 10, 64)
	now := a.nowMS()
	return beat > 0 && now-beat <= BarFreshMS && beat <= now+BarFreshMS, m[store.KeyBarNotify], nil
}

// BarReady reports whether trackd should hand notifications to the menu
// bar app: it is running and may notify.
func (a *App) BarReady() bool {
	running, access, err := a.BarState()
	return err == nil && running && access == NotifyGranted
}
