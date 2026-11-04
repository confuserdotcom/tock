package classify

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bounds of the screen keywords: per read, and per span over all its reads.
const (
	MaxWordsPerRead = 32
	MaxWordsPerSpan = 48
	minWordRunes    = 3
	maxWordRunes    = 24
)

// Keywords reduces text read on screen to its most frequent plain words:
// lowercase letters only, 3 to 24 letters, no stop words, nothing from a
// field holding a digit, an email or a URL, nothing that looks like a token
// or hex, and nothing a deny pattern matches. At most MaxWordsPerRead, most
// frequent first, ties alphabetical. It is pure.
func Keywords(lines []string, deny []*regexp.Regexp) []string {
	n := map[string]int{}
	for _, line := range lines {
		for _, field := range strings.Fields(line) {
			if dropField(field) {
				continue
			}
			for _, w := range strings.FieldsFunc(field, func(r rune) bool { return !unicode.IsLetter(r) }) {
				if tokenLike(w) {
					continue
				}
				w = strings.ToLower(w)
				if c := utf8.RuneCountInString(w); c < minWordRunes || c > maxWordRunes || stopWords[w] || hexLike(w) {
					continue
				}
				if slices.ContainsFunc(deny, func(re *regexp.Regexp) bool { return re.MatchString(w) }) {
					continue
				}
				n[w]++
			}
		}
	}
	out := make([]string, 0, len(n))
	for w := range n {
		out = append(out, w)
	}
	slices.SortFunc(out, func(a, b string) int {
		if n[a] != n[b] {
			return n[b] - n[a]
		}
		return strings.Compare(a, b)
	})
	if len(out) > MaxWordsPerRead {
		out = out[:MaxWordsPerRead]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MergeWords is add then the words of bag not in add, at most
// MaxWordsPerSpan: the latest read comes first and old words fall off.
func MergeWords(bag, add []string) []string {
	out := make([]string, 0, min(len(bag)+len(add), MaxWordsPerSpan))
	seen := map[string]bool{}
	for _, w := range slices.Concat(add, bag) {
		if seen[w] || len(out) == MaxWordsPerSpan {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// dropField: a whitespace field with a digit, an @, or a URL or domain shape
// is dropped whole, so no part of it is kept.
func dropField(f string) bool {
	if strings.ContainsAny(f, "0123456789@") || strings.Contains(f, "://") || strings.HasPrefix(strings.ToLower(f), "www.") {
		return true
	}
	// a.com, c.io/path: a dot between letters
	for i := 1; i+1 < len(f); i++ {
		if f[i] == '.' && isASCIILetter(f[i-1]) && isASCIILetter(f[i+1]) {
			return true
		}
	}
	return false
}

func isASCIILetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

// tokenLike: mixed case past the first letter (aZkQp, getUserName) or a
// long run of consonants (qwrtzxcvb), as random ids and keys have.
func tokenLike(w string) bool {
	rs := []rune(w)
	upper, lower := 0, 0
	for i, r := range rs {
		if unicode.IsUpper(r) {
			if i > 0 {
				upper++
			}
		} else {
			lower++
		}
	}
	if upper > 0 && lower > 0 {
		return true
	}
	run := 0
	for _, r := range strings.ToLower(w) {
		if strings.ContainsRune("aeiouyàâäéèêëîïôöùûüœæ", r) {
			run = 0
			continue
		}
		if run++; run >= 6 {
			return true
		}
	}
	return false
}

// hexLike: eight or more letters, all of them hex digits.
func hexLike(w string) bool {
	return len(w) >= 8 && strings.Trim(w, "abcdef") == ""
}

var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
the and for are but not you all any can had her was one our out has him his how its may new now old see two way who
did get got let put say she too use with that this from they will would there their what about which when your have
been were more into than then them these some could other only also just like over such very what where while here
les des une est pas que qui dans pour sur par avec son ses aux mais ont été sont cette ces nous vous ils elles
leur leurs comme plus tout tous fait faire être avoir elle lui même aussi bien peut donc car très entre sans sous
après avant encore alors était quand`) {
		stopWords[w] = true
	}
}
