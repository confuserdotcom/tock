package app

import (
	"fmt"

	"tock/internal/awimport"
	"tock/internal/store"
)

// AWMachine is the machine name imported ActivityWatch history goes under.
const AWMachine = "activitywatch"

// ImportResult is what an import read.
type ImportResult struct {
	Views, Days int
}

// ImportActivityWatch stores an ActivityWatch export as views of the machine
// AWMachine, one day at a time; a day imported again is replaced. This
// machine's rules label them. ActivityWatch already left out time away, so
// its views are never cut as stale.
func (a *App) ImportActivityWatch(data []byte) (ImportResult, error) {
	var r ImportResult
	days, err := awimport.Parse(data, a.now().Location())
	if err != nil {
		return r, err
	}
	now := a.nowMS()
	for key, val := range map[string]string{RemoteInputKey(AWMachine): "1", RemoteInputSinceKey(AWMachine): "", RemoteHIDKey(AWMachine): "1"} {
		if err := a.st.SetState(key, val, now); err != nil {
			return r, fmt.Errorf("app: import activitywatch: %w", err)
		}
	}
	for _, d := range days {
		vs := make([]store.RemoteView, len(d.Views))
		for i, v := range d.Views {
			if a.cfg.Privacy.Hides(v.Bundle, v.URL) {
				v.Title, v.URL = nil, nil // [privacy] hide_apps/hide_sites, as trackd does
			}
			vs[i] = store.RemoteView{StartMS: v.StartMS, EndMS: v.EndMS, Bundle: v.Bundle, App: v.App, Title: v.Title, URL: v.URL}
		}
		if err := a.st.ReplaceRemoteDay(AWMachine, d.Key, store.RemoteFile{MtimeMS: now, Size: int64(len(vs))}, vs); err != nil {
			return r, fmt.Errorf("app: import activitywatch: %w", err)
		}
		r.Views += len(vs)
		r.Days++
	}
	return r, nil
}
