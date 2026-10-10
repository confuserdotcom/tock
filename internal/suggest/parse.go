package suggest

import (
	"encoding/json"
	"fmt"
	"strings"

	"tock/internal/classify"
)

// Parse reads the suggest tool input for b. It returns one Suggestion per
// key of b, in id order: unknown ids are dropped, names are matched
// case-insensitively and returned in their declared spelling, unknown names
// and archived projects become "", and a key the model skipped gets an
// empty Suggestion (cached as "none").
func Parse(raw json.RawMessage, b Batch, rs *classify.Ruleset) ([]Suggestion, error) {
	var in struct {
		Suggestions []struct {
			ID       string  `json:"id"`
			Category *string `json:"category"`
			Project  *string `json:"project"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("suggest: bad tool input: %w", err)
	}
	got := map[string]Suggestion{}
	for _, s := range in.Suggestions {
		key, ok := b.Keys[s.ID]
		if !ok {
			continue
		}
		got[s.ID] = Suggestion{Key: key, Category: category(rs, deref(s.Category)), Project: project(rs, deref(s.Project))}
	}
	out := make([]Suggestion, 0, len(b.Keys))
	for i := 1; i <= len(b.Keys); i++ {
		id := fmt.Sprintf("k%d", i)
		s, ok := got[id]
		if !ok {
			s = Suggestion{Key: b.Keys[id]}
		}
		out = append(out, s)
	}
	return out, nil
}

// Canon returns the declared spellings of a cached suggestion, or false when
// it is empty or names a category or project the rules no longer offer.
func Canon(rs *classify.Ruleset, cat, proj string) (classify.Want, bool) {
	w := classify.Want{Category: category(rs, cat), Project: project(rs, proj)}
	if (cat != "" && w.Category == "") || (proj != "" && w.Project == "") || w == (classify.Want{}) {
		return classify.Want{}, false
	}
	return w, true
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func category(rs *classify.Ruleset, s string) string {
	for _, c := range rs.Categories {
		if s != "" && strings.EqualFold(c.Name, s) {
			return c.Name
		}
	}
	return ""
}

func project(rs *classify.Ruleset, s string) string {
	for _, p := range rs.Projects {
		if s != "" && !p.Archived && strings.EqualFold(p.Name, s) {
			return p.Name
		}
	}
	return ""
}
