// Package plan reads and edits a todo.txt file for the plan tab: one task
// per line, "x DATE " when done, "(A) " for a priority, +project, @context
// and key:value tags (due:, t:, id:). Every edit re-reads the file and acts
// on the line by its text, so a planner rewriting the file between two
// edits never makes tock tick the wrong line.
package plan

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Item is one parsed line.
type Item struct {
	Raw       string
	Done      bool
	DoneOn    string // YYYY-MM-DD, "" when done without a date
	Pri       string // "A".."Z", "" = none (for a done line: its pri: tag)
	Text      string // the words, without priority, dates and key:value tags
	Projects  []string
	Contexts  []string
	Due       string // due:YYYY-MM-DD
	Threshold string // t:YYYY-MM-DD, hidden before that day
	ID        string // id:…
}

var (
	doneRE = regexp.MustCompile(`^x (?:(\d{4}-\d{2}-\d{2}) )?`)
	priRE  = regexp.MustCompile(`^\(([A-Z])\) `)
	dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} `)
	kvRE   = regexp.MustCompile(`^([A-Za-z][\w-]*):(\S+)$`)
)

// Parse reads one line.
func Parse(raw string) Item {
	it := Item{Raw: raw}
	s := raw
	if m := doneRE.FindStringSubmatch(s); m != nil {
		it.Done, it.DoneOn, s = true, m[1], s[len(m[0]):]
	}
	if m := priRE.FindStringSubmatch(s); m != nil {
		it.Pri, s = m[1], s[len(m[0]):]
	}
	s = dateRE.ReplaceAllString(s, "") // creation date
	if it.Done {
		s = dateRE.ReplaceAllString(s, "") // "x DONE CREATED text"
	}
	var words []string
	for _, w := range strings.Fields(s) {
		switch m := kvRE.FindStringSubmatch(w); {
		case strings.HasPrefix(w, "+") && len(w) > 1:
			it.Projects = append(it.Projects, w[1:])
		case strings.HasPrefix(w, "@") && len(w) > 1:
			it.Contexts = append(it.Contexts, w[1:])
		case m != nil && !strings.Contains(m[2], "/"): // not a URL
			switch m[1] {
			case "due":
				it.Due = m[2]
			case "t":
				it.Threshold = m[2]
			case "id":
				it.ID = m[2]
			case "pri":
				if it.Pri == "" {
					it.Pri = m[2]
				}
			default:
				words = append(words, w)
			}
		default:
			words = append(words, w)
		}
	}
	it.Text = strings.Join(words, " ")
	return it
}

// Toggle marks a line done on day (dropping its priority into a pri: tag, as
// todo.txt asks) or not done (restoring the priority).
func Toggle(raw, day string) string {
	if m := doneRE.FindString(raw); m != "" {
		s := raw[len(m):]
		f := strings.Fields(s)
		pri := ""
		kept := f[:0]
		for _, w := range f {
			if v, ok := strings.CutPrefix(w, "pri:"); ok && len(v) == 1 {
				pri = v
				continue
			}
			kept = append(kept, w)
		}
		s = strings.Join(kept, " ")
		if pri != "" {
			s = "(" + pri + ") " + s
		}
		return s
	}
	s := raw
	if m := priRE.FindStringSubmatch(s); m != nil {
		s = s[len(m[0]):] + " pri:" + m[1]
	}
	return "x " + day + " " + s
}

// New is the line for a task typed in the add box: its priority first, then
// the creation date, then the rest.
func New(text, day string) string {
	text = strings.TrimSpace(text)
	if m := priRE.FindString(text); m != "" {
		return m + day + " " + strings.TrimSpace(text[len(m):])
	}
	return day + " " + text
}

// ErrGone is returned when the line to change is no longer in the file.
var ErrGone = errors.New("that line changed in the file meanwhile; refreshed")

// Read returns the file's lines without blank ones. A missing file is empty.
func Read(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// Write replaces the file atomically (a temp file renamed over it), so a
// sync tool or a reader never sees half a file.
func Write(path string, lines []string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	text := strings.Join(lines, "\n")
	if len(lines) > 0 {
		text += "\n"
	}
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if fi, err := os.Stat(path); err == nil {
		os.Chmod(tmp.Name(), fi.Mode().Perm())
	}
	return os.Rename(tmp.Name(), path)
}

// Edit re-reads path, replaces the first line equal to old by the result of
// f (nil removes it) and writes the file. old == "" appends f(""); at >= 0
// puts an appended line at that index instead (undoing a delete).
func Edit(path, old string, at int, f func(string) *string) error {
	lines, err := Read(path)
	if err != nil {
		return err
	}
	if old == "" {
		n := f("")
		if n == nil {
			return nil
		}
		if at >= 0 && at < len(lines) {
			lines = append(lines[:at], append([]string{*n}, lines[at:]...)...)
		} else {
			lines = append(lines, *n)
		}
		return Write(path, lines)
	}
	for i, l := range lines {
		if l != old {
			continue
		}
		if n := f(l); n != nil {
			lines[i] = *n
		} else {
			lines = append(lines[:i], lines[i+1:]...)
		}
		return Write(path, lines)
	}
	return ErrGone
}

// Index is the position of the first line equal to raw, or -1.
func Index(lines []string, raw string) int {
	for i, l := range lines {
		if l == raw {
			return i
		}
	}
	return -1
}

// Sections, in the order the tab shows them.
const (
	Overdue = iota
	Today
	Tomorrow
	Later
	NoDate
	Done
	NumSections
)

// SectionNames are the headings of the sections.
var SectionNames = [NumSections]string{"Overdue", "Today", "Tomorrow", "Later", "No date", "Done"}

// Row is one shown line: its item, its section and its place in the file.
type Row struct {
	Item
	Section int
	Line    int
}

// Rows sorts lines into sections for day (YYYY-MM-DD). Inside a section,
// lines go by due date, then by the time of day in a context (@07h30,
// @ven-07h30, @07:30), then in file order. Lines with t: after day are
// hidden; done lines show only when done on day (older ones are history).
func Rows(lines []string, day, tomorrow string) []Row {
	var out []Row
	for i, l := range lines {
		it := Parse(l)
		if s, ok := section(it, day, tomorrow); ok {
			out = append(out, Row{Item: it, Section: s, Line: i})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Section != b.Section {
			return a.Section < b.Section
		}
		if a.Due != b.Due {
			return a.Due < b.Due
		}
		return clock(a.Item) < clock(b.Item)
	})
	return out
}

var clockRE = regexp.MustCompile(`(?:^|\D)(\d{1,2})[h:](\d{2})$`)

// Clock is the minute of the day in a context (@07h30, @ven-07h30,
// @07:30); ok is false for an untimed line.
func Clock(it Item) (minute int, ok bool) {
	m := clock(it)
	return m, m < 24*60
}

// clock is the minute of the day in a context; untimed lines sort last.
func clock(it Item) int {
	for _, c := range it.Contexts {
		if m := clockRE.FindStringSubmatch(c); m != nil {
			h, _ := strconv.Atoi(m[1])
			mi, _ := strconv.Atoi(m[2])
			return h*60 + mi
		}
	}
	return 24 * 60
}

func section(it Item, day, tomorrow string) (int, bool) {
	switch {
	case it.Done:
		return Done, it.DoneOn == "" || it.DoneOn >= day
	case it.Threshold != "" && it.Threshold > day:
		return 0, false
	case it.Due == "":
		return NoDate, true
	case it.Due < day:
		return Overdue, true
	case it.Due == day:
		return Today, true
	case it.Due == tomorrow:
		return Tomorrow, true
	}
	return Later, true
}
