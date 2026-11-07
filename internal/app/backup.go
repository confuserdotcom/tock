package app

import (
	"errors"
	"os"
	"strconv"

	"tock/internal/snapshot"
	"tock/internal/span"
	"tock/internal/store"
)

// TopicIntegrity is the notice topic for a damaged database.
const TopicIntegrity = "integrity"

const dayMS = 24 * 3600 * 1000

// SnapshotDue reports whether trackd should take a daily snapshot now.
func (a *App) SnapshotDue() (bool, error) {
	if a.cfg.Backup.Keep <= 0 {
		return false, nil
	}
	list, err := snapshot.List(store.AutoDir(a.paths.DB))
	if err != nil {
		return false, err
	}
	var newest int64
	for _, i := range list {
		if i.Kind == snapshot.Daily && i.AtMS > newest {
			newest = i.AtMS
		}
	}
	b, open, err := a.st.OpenBundle()
	if err != nil {
		return false, err
	}
	away := !open || b == span.IdleBundleID
	return snapshot.Due(newest, a.nowMS(), away), nil
}

// DailySnapshot writes a daily snapshot, records the result in
// daemon_state and deletes what retention no longer keeps. A damaged
// database is recorded as such.
func (a *App) DailySnapshot() (snapshot.Info, int, error) {
	now := a.nowMS()
	stamp := strconv.FormatInt(now, 10)
	schema, err := a.st.SchemaVersion()
	if err != nil {
		return snapshot.Info{}, 0, err
	}
	var live int64
	if fi, err := os.Stat(a.paths.DB); err == nil {
		live = fi.Size()
	}
	dir := store.AutoDir(a.paths.DB)
	info, err := snapshot.Write(snapshot.Opts{
		Dir: dir, Name: snapshot.DailyName(now, schema), LiveSize: live,
		Free: snapshot.FreeDisk, Vacuum: a.st.BackupTo,
	})
	var dmg *snapshot.DamagedError
	switch {
	case errors.As(err, &dmg):
		_ = a.st.SetState(store.KeyIntegrityAt, stamp, now)
		_ = a.st.SetState(store.KeyIntegrityOK, dmg.Detail, now)
		_ = a.st.SetState(store.KeyBackupErr, err.Error(), now)
		return info, 0, err
	case err != nil:
		_ = a.st.SetState(store.KeyBackupErr, err.Error(), now)
		return info, 0, err
	}
	for _, kv := range [][2]string{{store.KeyBackupLast, stamp}, {store.KeyBackupErr, ""},
		{store.KeyIntegrityAt, stamp}, {store.KeyIntegrityOK, "ok"}} {
		if err := a.st.SetState(kv[0], kv[1], now); err != nil {
			return info, 0, err
		}
	}
	list, err := snapshot.List(dir)
	if err != nil {
		return info, 0, err
	}
	removed := 0
	for _, r := range snapshot.Plan(list, now, a.cfg.Backup.Keep) {
		if os.Remove(r.Path) == nil {
			removed++
		}
	}
	return info, removed, nil
}

// NoteDamage queues the damaged-database notice, at most once a day.
func (a *App) NoteDamage() error {
	now := a.nowMS()
	last, err := a.st.LastNoticeAt(TopicIntegrity, now-dayMS)
	if err != nil || last > 0 {
		return err
	}
	_, err = a.st.AddNotice(store.Notice{AtMS: now, Topic: TopicIntegrity,
		Title: "tock's database looks damaged",
		Body:  "Run tock doctor. Snapshots to restore from are listed by tock restore."})
	return err
}
