package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"tock/internal/app"
	"tock/internal/store"
)

// doctorCheck is one line of tock doctor.
type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

// doctorReport is tock doctor --json: the checks and what a maintainer
// asks first. It holds no titles, URLs or rule contents.
type doctorReport struct {
	Version       string        `json:"version"`
	OS            string        `json:"os"`
	Install       string        `json:"install"`
	TrackdVersion string        `json:"trackd_version,omitempty"`
	Schema        int           `json:"schema_version,omitempty"`
	SchemaBinary  int           `json:"schema_version_binary"`
	Compat        int           `json:"compat,omitempty"`
	CompatBinary  int           `json:"compat_binary"`
	Counters      *doctorCounts `json:"counters,omitempty"`
	Checks        []doctorCheck `json:"checks"`
}

type doctorCounts struct {
	WriteFails    int64  `json:"apply_failed_total"`
	LastError     string `json:"last_error,omitempty"`
	LastErrorMS   int64  `json:"last_error_ms,omitempty"`
	Dropped       int64  `json:"samples_dropped_total"`
	DroppedMS     int64  `json:"samples_dropped_ms,omitempty"`
	TrackdStopped string `json:"trackd_stopped,omitempty"`
	CapturePaused bool   `json:"capture_paused"`
	TrackdUp      bool   `json:"trackd_up"`
	LastSeenMS    int64  `json:"last_seen_ms,omitempty"`
	IntegrityOK   string `json:"integrity,omitempty"`
	BackupLastMS  int64  `json:"backup_last_ms,omitempty"`
	BackupError   string `json:"backup_error,omitempty"`
}

func doctorFacts(env Env, checks []doctorCheck) doctorReport {
	r := doctorReport{Version: version, OS: runtime.GOOS + "/" + runtime.GOARCH,
		Install: installMethod().String(), SchemaBinary: store.LatestVersion(),
		CompatBinary: store.CompatLevel, Checks: checks}
	if r.Checks == nil {
		r.Checks = []doctorCheck{}
	}
	st, err := openStore(env)
	if err != nil {
		return r
	}
	defer st.Close()
	r.Schema, _ = st.SchemaVersion()
	r.Compat, _ = st.Compat()
	if h, err := app.ReadHealth(st, env.Now().UnixMilli()); err == nil {
		r.TrackdVersion = h.Version
		r.Counters = &doctorCounts{WriteFails: h.WriteFails, LastError: h.LastError, LastErrorMS: h.LastErrorMS,
			Dropped: h.Dropped, DroppedMS: h.DroppedMS, TrackdStopped: h.TrackdStopped,
			CapturePaused: h.CapturePaused != "", TrackdUp: h.TrackdUp, LastSeenMS: h.LastSeenMS,
			IntegrityOK: h.IntegrityOK, BackupLastMS: h.BackupMS, BackupError: h.BackupErr}
	}
	return r
}

func writeDoctorJSON(env Env, checks []doctorCheck) error {
	b, err := json.MarshalIndent(doctorFacts(env, checks), "", "  ")
	if err != nil {
		return fmt.Errorf("doctor: %w", err)
	}
	_, err = fmt.Fprintln(env.Stdout, string(b))
	return err
}
