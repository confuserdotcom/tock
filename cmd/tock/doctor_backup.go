package main

import (
	"database/sql"
	"fmt"

	"tock/internal/app"
	"tock/internal/snapshot"
	"tock/internal/store"
)

// backupStaleMS: no daily snapshot for this long while trackd runs is a
// problem (it is due after 24-36 h).
const backupStaleMS = 48 * 3600_000

func ago(nowMS, atMS int64) string {
	h := (nowMS - atMS) / 3600_000
	if h < 1 {
		return "under an hour ago"
	}
	return fmt.Sprintf("%d h ago", h)
}

// doctorBackup reports trackd's daily snapshots and the last database check.
func doctorBackup(env Env, keep int, h app.Health, ok func(name, detail string), bad func(name, detail, fix string)) {
	now := env.Now().UnixMilli()
	list, _ := snapshot.List(store.AutoDir(env.dbPath()))
	var size int64
	for _, i := range list {
		size += i.Size
	}
	kept := fmt.Sprintf("%d kept, %s", len(list), sizeMB(size))
	switch {
	case h.BackupErr != "":
		bad("backup", h.BackupErr, "see "+store.AutoDir(env.dbPath()))
	case keep <= 0:
		ok("backup", "daily snapshots off ([backup] keep = 0)")
	case h.BackupMS == 0:
		ok("backup", "no daily snapshot yet")
	case h.TrackdUp && now-h.BackupMS > backupStaleMS:
		bad("backup", "last snapshot "+ago(now, h.BackupMS)+", "+kept, "check trackd's log")
	default:
		ok("backup", "last snapshot "+ago(now, h.BackupMS)+", "+kept)
	}
	switch {
	case h.Damaged():
		bad("database", h.IntegrityOK, "tock restore")
	case h.IntegrityMS > 0:
		ok("database", "integrity ok, checked "+ago(now, h.IntegrityMS))
	}
}

func sizeMB(b int64) string { return fmt.Sprintf("%.1f MB", float64(b)/(1<<20)) }

// doctorDeep runs the full integrity_check on the live database.
func doctorDeep(env Env, ok func(name, detail string), bad func(name, detail, fix string)) {
	db, err := sql.Open("sqlite", "file:"+env.dbPath()+"?mode=ro")
	if err != nil {
		bad("database", err.Error(), "tock restore")
		return
	}
	defer db.Close()
	var res string
	if err := db.QueryRow(`PRAGMA integrity_check(1)`).Scan(&res); err != nil {
		bad("database", err.Error(), "tock restore")
		return
	}
	if res != "ok" {
		bad("database", res, "tock restore")
		return
	}
	ok("database", "full integrity check ok")
}
