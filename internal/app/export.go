package app

import (
	"time"

	"tock/internal/report"
)

// Export returns the labelled spans overlapping [from, to), clipped to it;
// the open span ends now.
func (a *App) Export(from, to time.Time) ([]report.ExportRow, error) {
	vs, ls, err := a.labelled(from.UnixMilli(), to.UnixMilli(), a.nowMS())
	if err != nil {
		return nil, err
	}
	loc := a.now().Location()
	at := func(ms int64) time.Time { return time.UnixMilli(ms).In(loc).Truncate(time.Second) }
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	rows := make([]report.ExportRow, len(vs))
	for i, v := range vs {
		l := ls[i]
		rows[i] = report.ExportRow{ID: v.ID, Start: at(v.Start), End: at(v.End),
			App: v.App, BundleID: v.BundleID, Title: str(v.Title), URL: str(v.URL), Private: v.Private,
			Category: l.Category, Kind: l.Kind, Project: l.Project,
			CategorySource: string(l.CategorySource), ProjectSource: string(l.ProjectSource)}
		if l.Project != "" {
			rows[i].Rate = a.rs.Rate(l.Project)
		}
	}
	return rows, nil
}
