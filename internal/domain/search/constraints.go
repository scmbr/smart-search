package search

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
)

type QueryConstraint struct{ Field, Value, Text, Source string }

const countWords = `-?[0-9]{1,7}|одного|одной|один|одну|одна|двух|два|две|трех|три|четырех|четыре|пяти|пять|шести|шесть|семи|семь|восьми|восемь|девяти|девять|десяти|десять`

var quantityQuery = regexp.MustCompile(`(?:^|\s)(` + countWords + `)\s+(пользовател[ьяей]+|users?|пк|компьютер[аоыев]*|устройств[ао]*|участник[аоыев]*|ip(?:\s+адрес[аоыев]*)?)`)
var termQuery = regexp.MustCompile(`(?:^|\s)(` + countWords + `)\s+(месяц(?:а|ев)?|мес\.?|год(?:а)?|лет|months?|years?)`)
var bareYearQuery = regexp.MustCompile(`(?:^|\s)на\s+год`)
var perpetualQuery = regexp.MustCompile(`(?:^|\s)(бессрочн(?:о|ая|ый)|без ограничения срока|без ограничений по сроку)`)
var editionQuery = regexp.MustCompile(`(?:^|\s)(standard|enterprise|professional|advanced|solo|ultimate|family|personal|расширенн(?:ая|ый)|стандартн(?:ая|ый))`)
var explicitEditionQuery = regexp.MustCompile(`(?:^|\s)редакци[яи]\s+([\pL]+)`)
var homeBusinessQuery = regexp.MustCompile(`(?:^|\s)(для дома и бизнеса|home and business)`)
var homeStudentQuery = regexp.MustCompile(`(?:^|\s)(для дома и учебы|home and student)`)
var serverQuery = regexp.MustCompile(`(?:^|\s)для\s+(?:(?:физического|файлового|виртуального)\s+)?сервера`)
var workstationQuery = regexp.MustCompile(`(?:^|\s)для\s+рабочей\s+станции`)
var availabilityQuery = regexp.MustCompile(`(?:^|\s)в\s+наличии`)
var daysQuery = regexp.MustCompile(`(?:^|\s)(` + countWords + `)\s+(день|дня|дней|days?)`)
var quantityRange = regexp.MustCompile(`(?:от\s+[0-9]+\s+до\s+|[0-9]+\s*[-–/]\s*)[0-9]+\s+(?:пользовател|пк|устройств|месяц|год)`)
var negativeQuantity = regexp.MustCompile(`(?:^|\s)(?:(?:на|для|до)\s*)?-[0-9]+\s+(?:пользовател|users?|пк|устройств|участник|ip|месяц|мес|год|лет)`)
var negatedConstraint = regexp.MustCompile(`(?:^|\s)(?:не|без)\s+(?:на\s+)?(?:standard|enterprise|professional|advanced|family|personal|продлени[ея]|обновлени[ея]|поддержк[аи]|[0-9]+\s+(?:месяц|год|лет)|год)`)

func parseCount(s string) (int, error) {
	words := map[string]int{"один": 1, "одну": 1, "одна": 1, "одного": 1, "одной": 1, "два": 2, "две": 2, "двух": 2, "три": 3, "трех": 3, "четыре": 4, "четырех": 4, "пять": 5, "пяти": 5, "шесть": 6, "шести": 6, "семь": 7, "семи": 7, "восемь": 8, "восьми": 8, "девять": 9, "девяти": 9, "десять": 10, "десяти": 10}
	if n, ok := words[s]; ok {
		return n, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 1000000 {
		return 0, ErrInvalidQuery
	}
	return n, nil
}

func parseConstraints(r *Request) error {
	r.Constraints = nil
	r.UnsupportedConstraints = nil
	if r.TermMonths < 0 || r.TermMonths > 1200 || len(r.Edition) > 80 {
		return ErrInvalidQuery
	}
	if r.Edition != "" {
		r.Edition = catalog.CanonicalEdition(r.Edition)
		if r.Edition == "" {
			return ErrInvalidQuery
		}
	}
	if r.TermMonths > 0 {
		if r.TermKind != "" && r.TermKind != "fixed" {
			return ErrInvalidQuery
		}
		r.TermKind = "fixed"
	}
	if r.TermKind != "" && r.TermKind != "fixed" && r.TermKind != "perpetual" {
		return ErrInvalidQuery
	}
	if r.Deployment != "" && r.Deployment != "server" && r.Deployment != "workstation" {
		return ErrInvalidQuery
	}
	text := strings.ToLower(strings.ReplaceAll(r.Query, "ё", "е"))
	context := canonicalQuery(text)
	knownProduct := strings.Contains(context, "anydesk") || strings.Contains(context, "office") || strings.Contains(context, "кибер") || strings.Contains(context, "сканер")
	if quantityRange.MatchString(text) || negativeQuantity.MatchString(text) {
		return fmt.Errorf("%w: ambiguous quantity range", ErrInvalidQuery)
	}
	add := func(field, value, fragment string) {
		r.Constraints = append(r.Constraints, QueryConstraint{Field: field, Value: value, Text: strings.TrimSpace(fragment), Source: "query"})
	}
	unsupported := func(field, fragment string) {
		r.UnsupportedConstraints = append(r.UnsupportedConstraints, QueryConstraint{Field: field, Text: fragment, Source: "query"})
	}
	if negatedConstraint.MatchString(text) {
		unsupported("negated_constraint", text)
		return nil
	}
	set := func(field, value, fragment string, destination *string) error {
		if *destination != "" && *destination != value {
			return fmt.Errorf("%w: conflicting %s", ErrInvalidQuery, field)
		}
		*destination = value
		add(field, value, fragment)
		return nil
	}
	// Match offsets refer only to normalized text; OriginalQuery is never overwritten.
	apply := func(pattern *regexp.Regexp, consume func([]string) error) error {
		matches := pattern.FindAllStringSubmatchIndex(text, -1)
		var out strings.Builder
		last := 0
		for _, indices := range matches {
			end := indices[1]
			if end < len(text) {
				next, _ := utf8.DecodeRuneInString(text[end:])
				if unicode.IsLetter(next) || unicode.IsDigit(next) {
					continue
				}
			}
			var values []string
			for i := 0; i < len(indices); i += 2 {
				if indices[i] < 0 {
					values = append(values, "")
				} else {
					values = append(values, text[indices[i]:indices[i+1]])
				}
			}
			if err := consume(values); err != nil {
				return err
			}
			out.WriteString(text[last:indices[0]])
			out.WriteByte(' ')
			last = end
		}
		if last > 0 {
			out.WriteString(text[last:])
			text = out.String()
		}
		return nil
	}
	if strings.Contains(text, "не в наличии") || strings.Contains(text, "необязательно в наличии") || strings.Contains(text, "не обязательно в наличии") {
		unsupported("availability_negation", text)
	} else {
		if err := apply(availabilityQuery, func(m []string) error {
			r.Policy = "available_only"
			add("availability_policy", r.Policy, m[0])
			return nil
		}); err != nil {
			return err
		}
	}
	if strings.Contains(text, "не бессроч") {
		unsupported("term_negation", text)
	} else {
		if err := apply(perpetualQuery, func(m []string) error {
			if r.TermMonths > 0 {
				return ErrInvalidQuery
			}
			return set("term_kind", "perpetual", m[0], &r.TermKind)
		}); err != nil {
			return err
		}
	}
	if err := apply(termQuery, func(m []string) error {
		n, err := parseCount(m[1])
		if err != nil {
			return err
		}
		if strings.HasPrefix(m[2], "год") || m[2] == "лет" || strings.HasPrefix(m[2], "year") {
			n *= 12
		}
		if n > 1200 || (r.TermMonths > 0 && r.TermMonths != n) || r.TermKind == "perpetual" {
			return fmt.Errorf("%w: conflicting/invalid term", ErrInvalidQuery)
		}
		r.TermMonths = n
		r.TermKind = "fixed"
		add("term_months", strconv.Itoa(n), m[0])
		return nil
	}); err != nil {
		return err
	}
	if err := apply(bareYearQuery, func(m []string) error {
		if (r.TermMonths > 0 && r.TermMonths != 12) || r.TermKind == "perpetual" {
			return ErrInvalidQuery
		}
		r.TermMonths = 12
		r.TermKind = "fixed"
		add("term_months", "12", m[0])
		return nil
	}); err != nil {
		return err
	}
	if err := apply(daysQuery, func(m []string) error { unsupported("term_days", m[0]); return nil }); err != nil {
		return err
	}
	if err := apply(quantityQuery, func(m []string) error {
		n, err := parseCount(m[1])
		if err != nil {
			return err
		}
		if strings.HasPrefix(m[2], "пользовател") || strings.HasPrefix(m[2], "user") {
			if r.MinUsers > 0 && r.MinUsers != n {
				return fmt.Errorf("%w: conflicting users", ErrInvalidQuery)
			}
			r.MinUsers = n
			add("min_users", strconv.Itoa(n), m[0])
		} else {
			field := "min_devices"
			if strings.HasPrefix(m[2], "участник") {
				field = "min_participants"
			}
			if strings.HasPrefix(m[2], "ip") {
				field = "min_ip_addresses"
			}
			r.UnsupportedConstraints = append(r.UnsupportedConstraints, QueryConstraint{Field: field, Value: strconv.Itoa(n), Text: strings.TrimSpace(m[0]), Source: "query"})
		}
		return nil
	}); err != nil {
		return err
	}
	if err := apply(explicitEditionQuery, func(m []string) error {
		value := catalog.CanonicalEdition(m[1])
		if value == "" {
			unsupported("edition", m[0])
			return nil
		}
		return set("edition", value, m[0], &r.Edition)
	}); err != nil {
		return err
	}
	if knownProduct {
		if err := apply(editionQuery, func(m []string) error {
			value := catalog.CanonicalEdition(m[1])
			if value == "" {
				unsupported("edition", m[0])
				return nil
			}
			return set("edition", value, m[0], &r.Edition)
		}); err != nil {
			return err
		}
		if err := apply(homeBusinessQuery, func(m []string) error { return set("edition", "home_business", m[0], &r.Edition) }); err != nil {
			return err
		}
		if err := apply(homeStudentQuery, func(m []string) error { return set("edition", "home_student", m[0], &r.Edition) }); err != nil {
			return err
		}
	}
	if err := apply(serverQuery, func(m []string) error { return set("deployment", "server", m[0], &r.Deployment) }); err != nil {
		return err
	}
	if err := apply(workstationQuery, func(m []string) error { return set("deployment", "workstation", m[0], &r.Deployment) }); err != nil {
		return err
	}
	if len(r.Constraints) > 0 || len(r.UnsupportedConstraints) > 0 {
		residual := strings.Join(strings.Fields(text), " ")
		residual = strings.TrimSpace(strings.TrimSuffix(residual, " на"))
		if residual == "по" {
			return nil
		}
		if residual != "" {
			r.Query = residual
		} else {
			unsupported("product_intent_missing", r.Query)
		}
	}
	return nil
}
