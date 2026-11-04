package classify

// AutoPick is an inbox key tock may label on its own.
type AutoPick struct {
	Item
	Guess
}

// AutoPicks returns the inbox items whose guess is strong enough to apply
// without asking: a category, from the same site or app, holding at least
// minShare of the vote. Keys in done were auto-labelled before and are never
// picked again, so a correction sticks. Private keys are never picked.
// minShare <= 0 picks nothing.
func AutoPicks(items []Item, guesses map[string]Guess, done map[string]bool, minShare float64) []AutoPick {
	if minShare <= 0 {
		return nil
	}
	var out []AutoPick
	for _, it := range items {
		g, ok := guesses[it.Key]
		if !ok || it.Private || done[it.Key] || g.Category == "" || g.Share < minShare ||
			(g.Reason != GuessSite && g.Reason != GuessApp) {
			continue
		}
		out = append(out, AutoPick{Item: it, Guess: g})
	}
	return out
}
