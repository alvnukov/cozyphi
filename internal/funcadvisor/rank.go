package funcadvisor

import (
	"cmp"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var words = regexp.MustCompile(`[\pL\pN_$]+`)

// Task vocabulary bridges common coding requests in the two languages used
// by the harness. Package-specific aliases may be supplied in its documentation.
var vocabulary = map[string]string{
	"сборк": "build", "сборо": "build", "пакет": "package", "упав": "fail", "пада": "fail",
	"паден": "fail", "ошиб": "error", "сбой": "fail", "неуда": "fail", "слома": "fail",
	"провал": "fail", "отмен": "cancel", "уникал": "unique", "повтор": "duplicate",
	"чтен": "read", "чита": "read", "прочит": "read", "искать": "search", "поиск": "search",
	"найди": "search", "файло": "file", "файл": "file", "запис": "write", "измен": "edit",
	"команд": "command", "тест": "test", "прове": "check", "счёт": "count", "счет": "count",
	"колич": "count", "посчит": "count", "число": "count", "вывед": "list", "спис": "list",
}
var stopwords = map[string]bool{
	"the": true, "with": true, "from": true, "for": true, "each": true, "are": true, "this": true, "and": true, "how": true, "can": true,
	"которые": true, "который": true, "для": true, "как": true, "это": true, "при": true, "этот": true, "если": true,
}

func normalized(word string) string {
	word = strings.ToLower(word)
	// Prefer the longest matching prefix ("файло" over "файл").
	best, value := 0, word
	for prefix, alias := range vocabulary {
		if len(prefix) > best && strings.HasPrefix(word, prefix) {
			best, value = len(prefix), alias
		}
	}
	if best > 0 {
		return value
	}
	switch word {
	case "failed", "failing", "failures", "failure":
		return "fail"
	case "counting", "counts":
		return "count"
	case "cancelled", "canceled":
		return "cancel"
	case "searching", "find", "grep":
		return "search"
	}
	if len(word) > 4 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") {
		return word[:len(word)-1]
	}
	return word
}

func terms(text string) map[string]bool {
	result := make(map[string]bool)
	for _, word := range words.FindAllString(text, -1) {
		var part []rune
		prevLower := false
		add := func() {
			w := normalized(string(part))
			if len([]rune(w)) >= 2 && !stopwords[w] {
				result[w] = true
			}
			part = part[:0]
		}
		for _, r := range word {
			if r == '_' || r == '$' {
				add()
				prevLower = false
				continue
			}
			if prevLower && unicode.IsUpper(r) {
				add()
			}
			part = append(part, r)
			prevLower = unicode.IsLower(r)
		}
		add()
	}
	return result
}

type posting struct {
	position int
	weight   float64
}
type index struct {
	cards    []Card
	postings map[string][]posting
	exact    map[string][]int
}

func newIndex(cards []Card) *index {
	x := &index{cards: cards, postings: make(map[string][]posting), exact: make(map[string][]int)}
	for i, c := range cards {
		weights := make(map[string]float64)
		for _, field := range []struct {
			s string
			w float64
		}{{c.ID, 1}, {c.Purpose, 1}, {c.Test, 2}, {c.Signature, 2}, {c.Name, 5}} {
			fieldTerms := terms(field.s)
			// A short exact-purpose name conveys more than a passing mention in a long description.
			weight := field.w / math.Sqrt(float64(max(len(fieldTerms), 1)))
			for term := range fieldTerms {
				weights[term] = max(weights[term], weight)
			}
		}
		for term, w := range weights {
			x.postings[term] = append(x.postings[term], posting{i, w})
		}
		key := strings.ToLower(c.Name)
		x.exact[key] = append(x.exact[key], i)
	}
	return x
}

type match struct {
	card  Card
	score float64
	exact bool
}

func (x *index) rank(query string) []match {
	scores := make(map[int]float64)
	// Accumulate in a stable order so equally ranked results and cache keys
	// are deterministic even when the words have different IDF weights.
	keys := make([]string, 0)
	for term := range terms(query) {
		keys = append(keys, term)
	}
	slices.Sort(keys)
	for _, k := range keys {
		hits := x.postings[k]
		idf := math.Log1p(float64(len(x.cards)) / float64(len(hits)+1))
		for _, p := range hits {
			scores[p.position] += idf * p.weight
		}
	}
	exact := make(map[int]bool)
	for _, term := range words.FindAllString(strings.ToLower(query), -1) {
		for _, id := range x.exact[term] {
			exact[id] = true
			scores[id] += 1000
		}
	}
	r := make([]match, 0, len(scores))
	for id, score := range scores {
		r = append(r, match{x.cards[id], score, exact[id]})
	}
	slices.SortFunc(r, func(a, b match) int {
		if n := cmp.Compare(b.score, a.score); n != 0 {
			return n
		}
		return cmp.Compare(a.card.ID, b.card.ID)
	})
	return r
}
