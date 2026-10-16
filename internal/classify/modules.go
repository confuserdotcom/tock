package classify

import (
	"strings"
	"unicode"
)

// A module is a project filed under a category (`category = "Study"`):
// tock finds it by its name and aliases in window titles and terminal
// directories, before any rule, and its time takes the module's category.
type module struct {
	project, category string
	keys              []string // foldWords of the name and each alias, " "-padded
}

// latin folds the accented Latin letters titles use to their base letters.
var latin = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae",
	'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ì': "i", 'í': "i",
	'î': "i", 'ï': "i", 'ñ': "n", 'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o",
	'ö': "o", 'ø': "o", 'œ': "oe", 'ù': "u", 'ú': "u", 'û': "u", 'ü': "u",
	'ý': "y", 'ÿ': "y", 'ß': "ss",
}

// foldWords lowers s, folds accents and keeps letters and digits as words
// separated by single spaces, with a space at each end: "Algèbre 1 –
// TD" becomes " algebre 1 td ". A key is found in a title when its
// foldWords is a substring of the title's, so words never match inside
// longer ones.
func foldWords(s string) string {
	var b strings.Builder
	b.WriteByte(' ')
	space := true
	for _, r := range strings.ToLower(s) {
		if f, ok := latin[r]; ok {
			b.WriteString(f)
			space = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	if !space {
		b.WriteByte(' ')
	}
	return b.String()
}

// moduleFor returns the first module (in rules.toml order) named in title
// or dir, or nil.
func (rs *Ruleset) moduleFor(title, dir string) *module {
	if len(rs.modules) == 0 || (title == "" && dir == "") {
		return nil
	}
	hay := foldWords(title) + foldWords(dir)
	for i := range rs.modules {
		for _, k := range rs.modules[i].keys {
			if strings.Contains(hay, k) {
				return &rs.modules[i]
			}
		}
	}
	return nil
}

type moduleKey struct{ key, project string }

// ModuleOf returns the module a name or alias (case and accents aside)
// names, or "". The plan's +tags use it.
func (rs *Ruleset) ModuleOf(name string) string {
	k := foldWords(name)
	if strings.TrimSpace(k) == "" {
		return ""
	}
	for _, mk := range rs.moduleKeys {
		if mk.key == k {
			return mk.project
		}
	}
	return ""
}

// ProjectCategory returns the category a project is filed under, or "".
func (rs *Ruleset) ProjectCategory(project string) string {
	return rs.projByLower[strings.ToLower(project)].Category
}
