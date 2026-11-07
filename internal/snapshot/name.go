// Package snapshot keeps compressed, checked copies of tock.db: one before
// each migration and a rolling set of daily ones. Names, retention and the
// due rule are pure; Write, List and Extract touch files.
package snapshot

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Kind says why a snapshot was taken.
type Kind string

const (
	Daily Kind = "daily"
	Pre   Kind = "pre" // before a migration
)

// Info is one snapshot file. Schema is the schema version of the data in
// it; To is the version a Pre snapshot was taken before migrating to.
type Info struct {
	Name, Path string
	Kind       Kind
	AtMS       int64
	Schema, To int
	Size       int64
}

const stampLayout = "20060102T150405Z"

var (
	dailyRE = regexp.MustCompile(`^daily-([0-9]{8}T[0-9]{6}Z)-v([0-9]{4})\.db\.gz$`)
	preRE   = regexp.MustCompile(`^pre-v([0-9]{4})-to-v([0-9]{4})\.db\.gz$`)
)

// DailyName is the file name of a daily snapshot taken at atMS.
func DailyName(atMS int64, schema int) string {
	return fmt.Sprintf("daily-%s-v%04d.db.gz", time.UnixMilli(atMS).UTC().Format(stampLayout), schema)
}

// PreName is the file name of the snapshot taken before migrating from to.
func PreName(from, to int) string {
	return fmt.Sprintf("pre-v%04d-to-v%04d.db.gz", from, to)
}

// Parse reads a snapshot file name. Anything else, temp files included, is
// not a snapshot.
func Parse(name string) (Info, bool) {
	if m := dailyRE.FindStringSubmatch(name); m != nil {
		at, err := time.Parse(stampLayout, m[1])
		if err != nil {
			return Info{}, false
		}
		v, _ := strconv.Atoi(m[2])
		return Info{Name: name, Kind: Daily, AtMS: at.UnixMilli(), Schema: v}, true
	}
	if m := preRE.FindStringSubmatch(name); m != nil {
		from, _ := strconv.Atoi(m[1])
		to, _ := strconv.Atoi(m[2])
		return Info{Name: name, Kind: Pre, Schema: from, To: to}, true
	}
	return Info{}, false
}
