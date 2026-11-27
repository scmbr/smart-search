package catalog

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func CanonicalEdition(s string) string {
	s = normalized(s)
	if value, ok := editions[s]; ok {
		return value
	}
	switch s {
	case "solo", "family", "personal", "extended", "ultimate", "home_business", "home_student":
		return s
	case "стандартная", "стандартный":
		return "standard"
	case "расширенная", "расширенный":
		return "extended"
	case "семейная", "для семьи":
		return "family"
	case "персональный":
		return "personal"
	case "home and business", "для дома и бизнеса":
		return "home_business"
	case "home and student", "для дома и учебы":
		return "home_student"
	}
	return ""
}

var fixedTerm = regexp.MustCompile(`^([1-9][0-9]{0,3})\s*(мес\.?|месяц(?:а|ев)?|months?|год|года|лет|years?)$`)

func offerTerm(parameters []Parameter) (TextAttribute, IntegerAttribute) {
	var kind, months collected
	for index, p := range parameters {
		if normalized(p.Name) != "срок действия" && normalized(p.Name) != "срок лицензии" {
			continue
		}
		if !kind.observe(p, index, "term.parameter") {
			continue
		}
		months.observe(p, index, "term.parameter")
		s := normalized(p.Value)
		if s == "бессрочная" || s == "бессрочно" || s == "perpetual" {
			kind.add("perpetual")
			continue
		}
		match := fixedTerm.FindStringSubmatch(s)
		if match == nil {
			kind.unrecognized = true
			months.unrecognized = true
			continue
		}
		n, _ := strconv.Atoi(match[1])
		if strings.HasPrefix(match[2], "год") || match[2] == "лет" || strings.HasPrefix(match[2], "year") {
			n *= 12
		}
		if n > 1200 {
			kind.unrecognized = true
			months.unrecognized = true
			continue
		}
		kind.add("fixed")
		months.add(strconv.Itoa(n))
	}
	k := kind.scalar()
	m := asInteger(months, "months")
	if k.Status == Conflict {
		m.Status = Conflict
		m.Value = 0
		m.Reason = "conflicting_term_kinds"
	}
	return k, m
}

var explicitEditionBefore = regexp.MustCompile(`(?:^|[^\pL])(standard|enterprise|professional|advanced|стандартная|расширенная)\s+редакци[яи](?:$|[^\pL])`)
var explicitEditionAfter = regexp.MustCompile(`(?:редакци[яи]|основная лицензия)\s+(standard|enterprise|professional|advanced|стандартная|расширенная)(?:$|[^\pL])`)
var anydeskEdition = regexp.MustCompile(`\b(solo|standard|advanced|ultimate)\b`)
var titleUsers = capacityPattern(`пользовател[ьяей]+|users?`)
var deviceCapacity = capacityPattern(`пк|компьютер[аоыев]*|устройств[ао]*|devices?`)
var ipCapacity = capacityPattern(`ip(?:[- ]адрес[аоыев]*)?`)
var participantCapacity = capacityPattern(`участник[аоыев]*|participants?`)
var ambiguousCapacity = regexp.MustCompile(`(?:от\s+[0-9]+\s+до\s+|[0-9]+\s*[-–/]\s*)[0-9]+\s+(?:пользовател|users?|пк|компьютер|устройств|devices?|ip|участник|participants?)|(?:^|[^\pL])(?:не|без)\s+(?:на\s+|до\s+)?[0-9]+`)
var officeHome2024 = regexp.MustCompile(`^microsoft\s+office\s+(?:для дома|home)\s+2024(?:$|[^\pL\pN])`)

func capacityPattern(unit string) *regexp.Regexp {
	return regexp.MustCompile(`(?:^|[^\pL\pN])(?:на|до)\s+(-?[0-9]+)\s+(?:` + unit + `)`)
}

func collectTitleCapacity(title string, pattern *regexp.Regexp, c *collected, rule string) {
	s := normalized(title)
	if ambiguousCapacity.MatchString(s) {
		return
	}
	for _, m := range pattern.FindAllStringSubmatchIndex(s, -1) {
		if m[1] < len(s) {
			next, _ := utf8.DecodeRuneInString(s[m[1]:])
			if unicode.IsLetter(next) || unicode.IsDigit(next) || next == '-' || strings.HasPrefix(strings.TrimSpace(s[m[1]:]), "/") {
				continue
			}
		}
		n, err := strconv.Atoi(s[m[2]:m[3]])
		if err != nil || n < 1 || n > 1000000 {
			c.unrecognized = true
			c.evidence = append(c.evidence, Evidence{Source: "title", Field: "name", RawValue: title, ParameterIndex: -1, RuleID: rule})
			continue
		}
		addTitleValue(c, strconv.Itoa(n), title, rule)
	}
}

func addTitleValue(c *collected, value, title, rule string) {
	c.add(value)
	c.evidence = append(c.evidence, Evidence{Source: "title", Field: "name", RawValue: title, ParameterIndex: -1, RuleID: rule})
}

func collectTitleConstraints(title string, edition, users *collected) {
	s := normalized(title)
	for _, pattern := range []*regexp.Regexp{explicitEditionBefore, explicitEditionAfter} {
		for _, match := range pattern.FindAllStringSubmatch(s, -1) {
			addTitleValue(edition, CanonicalEdition(match[1]), title, "edition.title.explicit")
		}
	}
	if strings.HasPrefix(s, "anydesk ") || strings.HasPrefix(s, "anydesk(") {
		for _, match := range anydeskEdition.FindAllStringSubmatch(s, -1) {
			addTitleValue(edition, match[1], title, "edition.title.anydesk")
		}
	}
	if strings.HasPrefix(s, "microsoft office ") {
		// Vendor-confirmed rename, bounded to Office Home 2024; see query-constraints.md.
		if officeHome2024.MatchString(s) {
			addTitleValue(edition, "home_student", title, "edition.title.office-home-2024")
		}
		if strings.Contains(s, "для дома и бизнеса") || strings.Contains(s, "home and business") {
			addTitleValue(edition, "home_business", title, "edition.title.office")
		}
		if strings.Contains(s, "для дома и учебы") || strings.Contains(s, "home and student") {
			addTitleValue(edition, "home_student", title, "edition.title.office")
		}
	}
	collectTitleCapacity(title, titleUsers, users, "users_limit.title.explicit")
}

var serverDeployment = regexp.MustCompile(`(?:^|[^\pL])(?:для\s+(?:(?:физического|файлового|виртуального)\s+)?сервера|серверная\s+лицензия|на\s+(?:одном\s+)?сервере)(?:$|[^\pL])`)
var workstationDeployment = regexp.MustCompile(`(?:^|[^\pL])(?:для\s+рабочей\s+станции|на\s+(?:одном\s+рабочем\s+месте|одной\s+рабочей\s+станции))(?:$|[^\pL])`)

func offerDeployment(title string) TextAttribute {
	var c collected
	s := normalized(title)
	if serverDeployment.MatchString(s) {
		addTitleValue(&c, "server", title, "deployment.title.server")
	}
	if workstationDeployment.MatchString(s) {
		addTitleValue(&c, "workstation", title, "deployment.title.workstation")
	}
	return c.scalar()
}
