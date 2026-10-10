package main

import (
	"log/slog"
	"strconv"
	"time"

	"tock/internal/collect"
	"tock/internal/config"
	"tock/internal/store"
)

const (
	// calendarCheckEvery is how often the calendar loop rereads the config
	// and the access state.
	calendarCheckEvery = time.Minute
	// calendarSyncEvery is how often events are read while access is
	// granted.
	calendarSyncEvery = 10 * 60_000
	// calendarBackMS and calendarAheadMS bound the sync window around now.
	calendarBackMS  = 48 * 3600_000
	calendarAheadMS = 24 * 3600_000
)

// Calendar reads calendar events; collect.EventKit is the macOS one.
type Calendar interface {
	Status() string
	Request() string
	Events(from, to int64, calendars []string) ([]collect.CalendarEvent, error)
}

// calendarLoop copies calendar events into the event table while
// config.toml [calendar] enabled is true. Logs carry the access state and
// event counts, never titles.
type calendarLoop struct {
	cal       Calendar
	st        *store.Store
	cfgPath   string
	canPrompt bool // trackd runs under launchd, so a prompt is attributed to it
	log       *slog.Logger
	now       func() int64

	asked      bool
	status     string // last recorded; "" = none yet
	lastSyncMS int64
	lastCals   string // the calendars of the last sync
	lastErr    string
}

// tick checks the config and the access state, and syncs when a sync is
// due. It never stops trackd.
func (c *calendarLoop) tick() {
	cfg, err := config.Load(c.cfgPath)
	if err != nil {
		c.warn("calendar: config", err.Error())
		return
	}
	if !cfg.Calendar.Enabled {
		c.record(collect.CalendarOff)
		c.lastSyncMS = 0
		return
	}
	status := c.cal.Status()
	if status == collect.CalendarUndetermined && c.canPrompt && !c.asked {
		c.asked = true
		c.log.Info("calendar: asking for access")
		status = c.cal.Request()
	}
	c.record(status)
	if status != collect.CalendarGranted {
		return
	}
	now := c.now()
	cals := ""
	for _, n := range cfg.Calendar.Calendars {
		cals += n + "\n"
	}
	if c.lastSyncMS != 0 && now-c.lastSyncMS < calendarSyncEvery && cals == c.lastCals {
		return
	}
	from, to := now-calendarBackMS, now+calendarAheadMS
	evs, err := c.cal.Events(from, to, cfg.Calendar.Calendars)
	if err != nil {
		c.record(collect.CalendarError)
		c.warn("calendar: read failed", err.Error())
		return
	}
	rows := make([]store.Event, len(evs))
	for i, e := range evs {
		rows[i] = store.Event{StartMS: e.StartMS, EndMS: e.EndMS, Title: e.Title, Calendar: e.Calendar}
	}
	changed, err := c.st.SyncEvents(from, to, rows)
	if err != nil {
		c.warn("calendar: store", err.Error())
		return
	}
	c.lastSyncMS, c.lastCals, c.lastErr = now, cals, ""
	if err := c.st.SetState(store.KeyCalendarSync, strconv.FormatInt(now, 10), now); err != nil {
		c.warn("calendar: state", err.Error())
	}
	if changed {
		c.log.Info("calendar synced", "events", len(rows))
	}
}

// record writes the access state to daemon_state when it changes.
func (c *calendarLoop) record(status string) {
	if status == c.status {
		return
	}
	if err := c.st.SetState(store.KeyCalendarStatus, status, c.now()); err != nil {
		c.warn("calendar: state", err.Error())
		return
	}
	c.status = status
	if status == collect.CalendarDenied {
		c.log.Warn("calendar access denied", "settings", "System Settings > Privacy & Security > Calendars > trackd")
	} else {
		c.log.Info("calendar status", "status", status)
	}
}

func (c *calendarLoop) warn(msg, detail string) {
	if s := msg + ": " + detail; s != c.lastErr {
		c.lastErr = s
		c.log.Warn(msg, "err", detail)
	}
}

// startCalendar runs the loop, first at once and then every
// calendarCheckEvery, until stop is called.
func startCalendar(c *calendarLoop, every time.Duration) (stop func()) {
	t := time.NewTicker(every)
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		c.tick()
		for {
			select {
			case <-t.C:
				c.tick()
			case <-done:
				return
			}
		}
	}()
	return func() {
		t.Stop()
		close(done)
		<-finished
	}
}
