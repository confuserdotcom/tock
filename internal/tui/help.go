package tui

var helpText = []string{
	"tock — keys",
	"",
	"  1 2 3 4 / tab    today · week · inbox · focus",
	"  r                refresh now (also every 5 s)",
	"  ?                today: score breakdown (? again for keys) · other tabs: keys",
	"  q / ctrl+c       quit",
	"",
	"  today            j/k move · g/G first/last · ctrl+d/u half page",
	"                   h/l previous/next day · . today · / filter",
	"                   t tag row · p project only · T tag and learn a rule",
	"  week             j/k day · h/l week · . this week · enter open day",
	"  inbox            enter learn a rule · a accept suggestion · o tag once · x ignore · u undo",
	"  focus            s start a session (presets or custom) · x stop",
	"  pickers          type a prefix · ↑/↓ · enter · esc",
	"",
	"  colours: bright deep · gray shallow · dark neutral · red distraction and unlabelled",
}

func helpBody(w int) []string {
	out := make([]string, len(helpText))
	for i, l := range helpText {
		if i == 0 {
			out[i] = " " + sHead.Render(l)
			continue
		}
		out[i] = " " + fit(l, w-1)
	}
	return out
}
