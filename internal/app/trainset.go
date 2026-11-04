package app

import (
	"encoding/json"
	"time"

	"tock/internal/classify"
	"tock/internal/store"
)

// trainName is the derived row of a closed day's training part; the
// number is its format.
func trainName(date string) string { return "train1:" + date }

// trainingSet is the local model's training set over all history (see
// classify.TrainingSet), merged from one part per day: closed days' parts
// come from disk while their key (dayKeys) holds, the others are labelled
// now and the closed ones kept.
func (a *App) trainingSet() ([]classify.Example, error) {
	first, ok, err := a.st.FirstMS()
	if err != nil || !ok {
		return nil, err
	}
	today := a.midnight(a.now())
	var days []time.Time
	for d := a.midnight(time.UnixMilli(first)); !d.After(today); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	keys := a.dayKeys(days[:len(days)-1])
	parts := make(map[string][]classify.TrainEntry, len(days))
	if keys != nil {
		names := make([]string, 0, len(keys))
		for date := range keys {
			names = append(names, trainName(date))
		}
		rows, err := a.st.DerivedRows(names)
		if err == nil {
			for date, key := range keys {
				r, ok := rows[trainName(date)]
				var p []classify.TrainEntry
				if ok && r.Key == key && json.Unmarshal(r.Data, &p) == nil {
					parts[date] = p
				}
			}
		}
	}
	var missing []time.Time
	for _, d := range days {
		if _, ok := parts[dateKey(d)]; !ok {
			missing = append(missing, d)
		}
	}
	if len(missing) > 0 {
		rows, ovs, err := a.spans(missing[0].UnixMilli(), missing[len(missing)-1].AddDate(0, 0, 1).UnixMilli())
		if err != nil {
			return nil, err
		}
		now := a.nowMS()
		closedBy := now - classify.MaxAFKMS
		for _, r := range rows {
			if r.End == nil {
				closedBy = min(closedBy, r.Start)
			}
		}
		ends := rowEnds(rows, now)
		save := map[string]store.Derived{}
		cut := a.cut // labelling here is not a report: its cuts are not counted
		for _, d := range missing {
			from, to := d.UnixMilli(), d.AddDate(0, 0, 1).UnixMilli()
			vs, err := a.decorate(a.base(rowsFor(rows, ends, from, to), ovs, from, to, now), ovs, from, to)
			if err != nil {
				a.cut = cut
				return nil, err
			}
			p := a.rs.TrainingPart(vs, a.rs.LabelAll(vs, ovs))
			parts[dateKey(d)] = p
			if key := keys[dateKey(d)]; key != "" && to <= closedBy {
				if b, err := json.Marshal(p); err == nil {
					save[trainName(dateKey(d))] = store.Derived{Key: key, Data: b}
				}
			}
		}
		a.cut = cut
		if len(save) > 0 {
			_ = a.st.PutDerived(save, now) // a cache: a failed write costs a later run the work
		}
	}
	ordered := make([][]classify.TrainEntry, 0, len(days))
	for _, d := range days {
		ordered = append(ordered, parts[dateKey(d)])
	}
	return classify.MergeTraining(ordered...), nil
}
