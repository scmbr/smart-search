package search

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const QueryPolicyVersion = "query-v3"

type QueryTerm struct {
	Text      string
	Fuzziness int
}

type QueryVariant struct {
	Text, Reason string
	Terms        []QueryTerm
}

type QueryPlan struct {
	Version     string
	ExactOnly   bool
	LimitReason string
	Variants    []QueryVariant
}

// Alias phrases are word-boundary replacements, never rules for individual typos.
var queryAliases = [][2]string{
	{"астра линукс", "astra linux"}, {"астралинукс", "astra linux"},
	{"крипто про", "криптопро"}, {"цсп", "csp"},
	{"маленького офиса", "small office"}, {"малого офиса", "small office"},
	{"касперский", "kaspersky"}, {"каспер", "kaspersky"},
	{"анидеск", "anydesk"}, {"энидеск", "anydesk"},
	{"майкрософт", "microsoft"}, {"майков", "microsoft"}, {"офис", "office"},
	{"резервное копирование", "бэкап"}, {"операционка", "операционная система"},
}

func queryTokens(s string) []string {
	s = strings.ReplaceAll(strings.ToLower(s), "ё", "е")
	var result []string
	for _, field := range strings.Fields(s) {
		field = strings.Trim(field, ",;:!?\"()[]{}")
		if strings.ContainsFunc(field, unicode.IsDigit) {
			result = append(result, field)
		} else {
			result = append(result, strings.FieldsFunc(field, func(r rune) bool { return !unicode.IsLetter(r) })...)
		}
	}
	return result
}

func canonicalQuery(s string) string {
	s = " " + strings.Join(queryTokens(s), " ") + " "
	for _, alias := range queryAliases {
		s = strings.ReplaceAll(s, " "+alias[0]+" ", " "+alias[1]+" ")
	}
	return strings.TrimSpace(s)
}

func queryPlan(query, kind string) QueryPlan {
	p := QueryPlan{Version: QueryPolicyVersion}
	if len(strings.Fields(query)) == 1 && strings.ContainsFunc(query, unicode.IsDigit) {
		p.ExactOnly = true
		return p
	}
	base := strings.Join(queryTokens(query), " ")
	canonical := canonicalQuery(query)
	seen := map[string]bool{}
	add := func(text, reason string) {
		if seen[text] || text == "" || len(p.Variants) == 5 {
			return
		}
		seen[text] = true
		v := QueryVariant{Text: text, Reason: reason}
		for _, word := range queryTokens(text) {
			if queryStopWord(word, kind) {
				continue
			}
			term := QueryTerm{Text: word}
			if !strings.ContainsFunc(word, func(r rune) bool { return !unicode.IsLetter(r) }) {
				length := utf8.RuneCountInString(word)
				if length >= 4 {
					term.Fuzziness = 1
				}
				if length >= 6 {
					term.Fuzziness = 2
				}
			}
			v.Terms = append(v.Terms, term)
		}
		if len(v.Terms) > 16 {
			p.ExactOnly = true
			p.LimitReason = "query_expansion_term_limit"
			return
		}
		if len(v.Terms) > 0 {
			p.Variants = append(p.Variants, v)
		}
	}
	add(base, "normalized")
	add(canonical, "synonym")
	add(canonicalQuery(protectedTransform(base, keyboardVariant)), "keyboard_layout_hypothesis")
	add(canonicalQuery(protectedTransform(canonical, toLatin)), "transliteration_hypothesis")
	add(canonicalQuery(protectedTransform(canonical, toCyrillic)), "transliteration_hypothesis")
	if p.ExactOnly || len(p.Variants) == 0 {
		p.ExactOnly = true
		p.Variants = nil
	}
	return p
}

func protectedTransform(s string, transform func(string) string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if !strings.ContainsFunc(word, unicode.IsDigit) {
			words[i] = transform(word)
		}
	}
	return strings.Join(words, " ")
}

func queryStopWord(word, kind string) bool {
	switch word {
	case "для", "на", "и", "в", "с", "по", "for", "the", "of", "a", "an":
		return true
	}
	if kind != "" {
		switch word {
		case "продление", "продления", "renewal", "обновление", "upgrade", "crossgrade", "поддержка", "техподдержка", "support", "техническая", "новая", "новую", "полная", "лицензия", "лицензию", "версия", "new", "full", "license":
			return true
		}
	}
	return false
}

func keyboardVariant(s string) string {
	latin := []rune("qwertyuiopasdfghjklzxcvbnm")
	cyr := []rune("йцукенгшщзфывапролдячсмить")
	var hasLatin, hasCyr bool
	for _, r := range s {
		hasLatin = hasLatin || unicode.In(r, unicode.Latin)
		hasCyr = hasCyr || unicode.In(r, unicode.Cyrillic)
	}
	if hasLatin == hasCyr {
		return s
	}
	from, to := latin, cyr
	if hasCyr {
		from, to = cyr, latin
	}
	return strings.Map(func(r rune) rune {
		for i, key := range from {
			if r == key {
				return to[i]
			}
		}
		return r
	}, s)
}

func toLatin(s string) string {
	var out strings.Builder
	letters := map[rune]string{'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya"}
	for _, r := range s {
		if value, ok := letters[r]; ok {
			out.WriteString(value)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func toCyrillic(s string) string {
	letters := []string{"a", "а", "b", "б", "c", "к", "d", "д", "e", "е", "f", "ф", "g", "г", "h", "х", "i", "и", "j", "й", "k", "к", "l", "л", "m", "м", "n", "н", "o", "о", "p", "п", "q", "к", "r", "р", "s", "с", "t", "т", "u", "у", "v", "в", "w", "в", "x", "кс", "y", "ы", "z", "з"}
	// Short initialisms are letter-by-letter: MTS is not the phonetic word "мц".
	if utf8.RuneCountInString(s) <= 3 {
		return strings.NewReplacer(letters...).Replace(s)
	}
	digraphs := []string{"shch", "щ", "sch", "щ", "zh", "ж", "kh", "х", "ts", "ц", "ch", "ч", "sh", "ш", "yu", "ю", "ya", "я", "yo", "е"}
	replacer := strings.NewReplacer(append(digraphs, letters...)...)
	return replacer.Replace(s)
}
