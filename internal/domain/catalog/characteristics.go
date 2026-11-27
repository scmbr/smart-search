package catalog

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizationVersion must change when rules, evidence, or result semantics change.
const NormalizationVersion = "characteristics-v4"

type Knowledge string

const (
	Known    Knowledge = "known"
	Unknown  Knowledge = "unknown"
	Conflict Knowledge = "conflict"
)

type Evidence struct {
	Source         string
	Field          string
	RawValue       string
	ParameterIndex int // -1 for a title, otherwise the original parameter index.
	RuleID         string
}

type TextAttribute struct {
	Status   Knowledge
	Value    string
	Reason   string
	Evidence []Evidence
}

type SetAttribute struct {
	Status   Knowledge
	Values   []string
	Reason   string
	Evidence []Evidence
}

type IntegerAttribute struct {
	Status   Knowledge
	Value    int
	Unit     string
	Reason   string
	Evidence []Evidence
}

type Characteristics struct {
	PolicyVersion                                     string
	ProductType                                       TextAttribute
	OfferKind                                         TextAttribute
	LicenseType                                       TextAttribute
	Version                                           TextAttribute
	Edition                                           TextAttribute
	Platforms                                         SetAttribute
	Architectures                                     SetAttribute
	UsersLimit                                        IntegerAttribute
	DevicesLimit, IPAddressesLimit, ParticipantsLimit IntegerAttribute
	Nodes                                             IntegerAttribute
	TermKind                                          TextAttribute
	TermMonths                                        IntegerAttribute
	Deployment                                        TextAttribute
}

func normalized(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\ufeff' {
			return -1
		}
		return r
	}, s)
	return strings.ReplaceAll(strings.ToLower(strings.Join(strings.Fields(s), " ")), "ё", "е")
}

func unknownValue(s string) bool {
	switch normalized(s) {
	case "", "не определено", "другое или отсутствует", "другое", "нет данных", "не указано", "n/a":
		return true
	}
	return false
}

type collected struct {
	values       map[string]bool
	evidence     []Evidence
	unrecognized bool
}

func (c *collected) add(value string) {
	if c.values == nil {
		c.values = make(map[string]bool)
	}
	c.values[value] = true
}

func (c *collected) observe(p Parameter, index int, rule string) bool {
	raw := p.RawValue
	if raw == "" {
		raw = p.Value
	}
	c.evidence = append(c.evidence, Evidence{Source: "parameter", Field: p.Name, RawValue: raw, ParameterIndex: index, RuleID: rule})
	return p.Known && !unknownValue(p.Value)
}

func (c collected) sortedValues() []string {
	var values []string
	for value := range c.values {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func (c collected) scalar() TextAttribute {
	r := TextAttribute{Status: Unknown, Evidence: c.evidence, Reason: "missing"}
	if len(c.values) > 1 {
		r.Status = Conflict
		r.Reason = "contradictory_sources"
		return r
	}
	if c.unrecognized {
		r.Reason = "unrecognized"
		return r
	}
	if len(c.values) == 1 {
		r.Status = Known
		r.Value = c.sortedValues()[0]
		r.Reason = ""
	} else if len(c.evidence) > 0 {
		r.Reason = "source_unknown"
	}
	return r
}

func (c collected) set() SetAttribute {
	r := SetAttribute{Status: Unknown, Evidence: c.evidence, Reason: "missing"}
	if c.unrecognized {
		r.Reason = "unrecognized"
		return r
	}
	if len(c.values) > 0 {
		r.Status = Known
		r.Values = c.sortedValues()
		r.Reason = ""
	} else if len(c.evidence) > 0 {
		r.Reason = "source_unknown"
	}
	return r
}

var licenseTypes = map[string]string{
	"полная версия": "full", "full": "full", "full version": "full",
	"продление": "renewal", "renewal": "renewal",
	"обновление": "upgrade", "upgrade": "upgrade", "upg": "upgrade",
	"перекрестное обновление": "crossgrade", "crossgrade": "crossgrade",
	"техподдержка": "support", "техническая поддержка": "support", "support": "support", "maintenance;support": "support",
	"подписка": "subscription", "подписка csp": "subscription_csp",
	"модуль": "module", "модули": "module", "базовая": "base", "миграция": "migration",
	"расширение лицензии": "addon", "add-on": "addon", "лицензионный платеж": "license_fee",
	"esd": "esd", "дистрибутив": "distribution", "учебный комплект": "education_bundle", "аренда": "rental",
}

var editions = map[string]string{
	"standard": "standard", "enterprise": "enterprise", "professional": "professional",
	"смоленск": "smolensk", "воронеж": "voronezh", "орел": "orel",
	"максимальная": "ultimate", "plus": "plus", "advanced": "advanced", "premium": "premium",
	"расширенная": "extended", "lite": "lite",
}

var numericVersion = regexp.MustCompile(`^[vV]?([0-9]{1,4}(?:\.[0-9]{1,4}){0,3})$`)
var integerCapacity = regexp.MustCompile(`^[1-9][0-9]{0,6}$`)
var titleVersions = []struct {
	rule    string
	pattern *regexp.Regexp
}{
	{"version.title.microsoft-office", regexp.MustCompile(`^microsoft\s+office(?:\s+(?:для дома(?:\s+и\s+(?:бизнеса|учебы))?|home and business|home and student|home|professional|standard))?\s+(20[0-9]{2})`)},
	{"version.title.cryptopro-csp", regexp.MustCompile(`(?:^|[^\pL\pN])(?:крипто\s*про|cryptopro)\s+csp\s+(?:v|верси[яи]\s*)?([0-9]{1,4}(?:\.[0-9]{1,4}){1,3})`)},
	{"version.title.astra-linux", regexp.MustCompile(`(?:^|[^\pL\pN])astra\s+linux(?:\s+(?:special\s+edition|se))?\s+(?:v|верси[яи]\s*)?([0-9]{1,4}(?:\.[0-9]{1,4}){1,3})`)},
}

// Normalize never mutates the offer and never reads another SKU or description.
func Normalize(o Offer) Characteristics {
	var product, license, version, edition, platform, architecture, users, nodes collected
	for index, p := range o.Parameters {
		switch normalized(p.Name) {
		case "тип продукта":
			if product.observe(p, index, "product_type.parameter") {
				switch normalized(p.Value) {
				case "софт", "software":
					product.add("software")
				case "электроника", "hardware":
					product.add("hardware")
				default:
					product.unrecognized = true
				}
			}
		case "тип лицензии":
			if license.observe(p, index, "license_type.parameter") {
				if value, ok := licenseTypes[normalized(p.Value)]; ok {
					license.add(value)
				} else {
					license.unrecognized = true
				}
			}
		case "версия":
			if !p.Known || unknownValue(p.Value) {
				version.observe(p, index, "version.parameter")
				continue
			}
			if match := numericVersion.FindStringSubmatch(strings.TrimSpace(p.Value)); match != nil {
				version.observe(p, index, "version.parameter.numeric")
				version.add(match[1])
			} else if value := CanonicalEdition(p.Value); value != "" {
				edition.observe(p, index, "edition.parameter")
				edition.add(value)
			} else {
				version.observe(p, index, "version.parameter")
				version.unrecognized = true
			}
		case "редакция":
			if edition.observe(p, index, "edition.parameter") {
				if value := CanonicalEdition(p.Value); value != "" {
					edition.add(value)
				} else {
					edition.unrecognized = true
				}
			}
		case "платформа", "совместимые платформы":
			collectSet(&platform, p, index, "platform.parameter", mapPlatform)
		case "совместимая архитектура/платформа", "архитектура":
			collectSet(&architecture, p, index, "architecture.parameter", mapArchitecture)
		case "к-во пользователей до", "количество пользователей":
			collectInteger(&users, p, index, "users_limit.parameter", "users")
		case "к-во узлов", "количество узлов":
			collectInteger(&nodes, p, index, "nodes.parameter", "nodes")
		}
	}
	productType := product.scalar()
	licenseType := license.scalar()
	role := offerRole(o.Name, productType, licenseType)
	if productType.Value != "hardware" && productType.Status != Conflict && role.Value != "upgrade" && licenseType.Value != "migration" && !titleTransition.MatchString(normalized(o.Name)) {
		title := normalized(o.Name)
		for _, rule := range titleVersions {
			for _, match := range rule.pattern.FindAllStringSubmatchIndex(title, -1) {
				// Check the right boundary without consuming it: the same comma
				// can introduce another product/version that must also be checked.
				if match[1] < len(title) {
					next, _ := utf8.DecodeRuneInString(title[match[1]:])
					if unicode.IsLetter(next) || unicode.IsNumber(next) || next == '.' {
						continue
					}
				}
				version.add(title[match[2]:match[3]])
				version.evidence = append(version.evidence, Evidence{Source: "title", Field: "name", RawValue: o.Name, ParameterIndex: -1, RuleID: rule.rule})
			}
		}
	}
	result := Characteristics{PolicyVersion: NormalizationVersion, ProductType: productType, OfferKind: role, LicenseType: licenseType,
		Version: version.scalar(), Edition: edition.scalar(), Platforms: platform.set(), Architectures: architecture.set(),
		UsersLimit: asInteger(users, "users"), Nodes: asInteger(nodes, "nodes")}
	if productType.Value != "hardware" && productType.Status != Conflict && role.Value != "upgrade" && licenseType.Value != "migration" && !titleTransition.MatchString(normalized(o.Name)) {
		collectTitleConstraints(o.Name, &edition, &users)
		result.Edition = edition.scalar()
		result.UsersLimit = asInteger(users, "users")
	}
	var devices, ips, participants collected
	if productType.Value != "hardware" && productType.Status != Conflict && role.Value != "upgrade" && licenseType.Value != "migration" && !titleTransition.MatchString(normalized(o.Name)) {
		collectTitleCapacity(o.Name, deviceCapacity, &devices, "devices_limit.title.explicit")
		collectTitleCapacity(o.Name, ipCapacity, &ips, "ip_addresses_limit.title.explicit")
		collectTitleCapacity(o.Name, participantCapacity, &participants, "participants_limit.title.explicit")
	}
	result.DevicesLimit = asInteger(devices, "devices")
	result.IPAddressesLimit = asInteger(ips, "ip_addresses")
	result.ParticipantsLimit = asInteger(participants, "participants")
	result.TermKind, result.TermMonths = offerTerm(o.Parameters)
	result.Deployment = offerDeployment(o.Name)
	return result
}

func mapPlatform(s string) string {
	switch normalized(s) {
	case "windows", "microsoft windows":
		return "windows"
	case "linux":
		return "linux"
	case "macos", "mac os", "mac os x":
		return "macos"
	case "android":
		return "android"
	case "ios":
		return "ios"
	}
	return ""
}

func mapArchitecture(s string) string {
	switch normalized(s) {
	case "x86-64", "x86_64", "amd64":
		return "x86_64"
	case "armv8", "aarch64", "arm64":
		return "arm64"
	case "эльбрус (vliw)", "elbrus (vliw)":
		return "elbrus_vliw"
	}
	return ""
}

func collectSet(c *collected, p Parameter, index int, rule string, lookup func(string) string) {
	if !c.observe(p, index, rule) {
		return
	}
	parts := strings.FieldsFunc(p.Value, func(r rune) bool { return r == ',' || r == ';' || r == '/' })
	if len(parts) == 0 {
		c.unrecognized = true
		return
	}
	for _, part := range parts {
		if value := lookup(part); value != "" {
			c.add(value)
		} else {
			c.unrecognized = true
		}
	}
	if len(c.values) == 0 {
		c.unrecognized = true
	}
}

func collectInteger(c *collected, p Parameter, index int, rule, unit string) {
	if !c.observe(p, index, rule) {
		return
	}
	if u := normalized(p.Unit); u != "" && !((unit == "users" && (u == "users" || u == "пользователей")) || (unit == "nodes" && (u == "nodes" || u == "узлов"))) {
		c.unrecognized = true
		return
	}
	s := strings.TrimSpace(p.Value)
	if !integerCapacity.MatchString(s) {
		c.unrecognized = true
		return
	}
	n, err := strconv.Atoi(s)
	if err != nil || n > 1000000 {
		c.unrecognized = true
		return
	}
	c.add(strconv.Itoa(n))
}

func asInteger(c collected, unit string) IntegerAttribute {
	s := c.scalar()
	r := IntegerAttribute{Status: s.Status, Reason: s.Reason, Evidence: s.Evidence, Unit: unit}
	if s.Status == Known {
		r.Value, _ = strconv.Atoi(s.Value)
	} // collectInteger only adds valid integers.
	return r
}

var titleRoleRules = []struct {
	role, rule string
	pattern    *regexp.Regexp
}{
	{"new_license", "role.title.new", regexp.MustCompile(`(?:^|[^\pL\pN])(?:новая лицензия|новую лицензию|полная версия|new license|full license)(?:$|[^\pL\pN])`)},
	{"renewal", "role.title.renewal", regexp.MustCompile(`(?:^|[^\pL\pN])(?:продлени[ея]|renewal)(?:$|[^\pL\pN])`)},
	{"upgrade", "role.title.upgrade", regexp.MustCompile(`(?:^|[^\pL\pN])(?:обновление|upgrade|crossgrade)(?:$|[^\pL\pN])`)},
}
var titleSupport = regexp.MustCompile(`(?:сертификат\s+(?:на\s+)?(?:технической поддержки|техподдержки)|support certificate|^(?:техническая поддержка|техподдержка|technical support|support)(?:$|[^\pL\pN])|^продление (?:технической поддержки|техподдержки)|[—–-]\s*(?:техническая поддержка|техподдержка|technical support)(?:$|[^\pL\pN]))`)
var negatedRenewal = regexp.MustCompile(`(?:без|не)\s+(?:автоматического\s+)?продления`)
var titleTransition = regexp.MustCompile(`(?:^|[^\pL\pN])(?:обновлени[ея]|upgrade|crossgrade|миграци[яи]|migration|переход)(?:$|[^\pL\pN])`)

func offerRole(title string, product, license TextAttribute) TextAttribute {
	c := collected{evidence: append([]Evidence(nil), license.Evidence...)}
	switch license.Value {
	case "full":
		c.add("new_license")
	case "renewal":
		c.add("renewal")
	case "upgrade", "crossgrade":
		c.add("upgrade")
	case "support":
		c.add("support")
	}
	if license.Reason == "unrecognized" {
		c.unrecognized = true
	}
	if product.Value != "hardware" && product.Status != Conflict {
		s := normalized(title)
		support := titleSupport.MatchString(s)
		if support {
			c.add("support")
			c.evidence = append(c.evidence, Evidence{Source: "title", Field: "name", RawValue: title, ParameterIndex: -1, RuleID: "role.title.support"})
		}
		for _, rule := range titleRoleRules {
			if rule.role == "renewal" && (support || negatedRenewal.MatchString(s)) {
				continue
			}
			if rule.pattern.MatchString(s) {
				c.add(rule.role)
				c.evidence = append(c.evidence, Evidence{Source: "title", Field: "name", RawValue: title, ParameterIndex: -1, RuleID: rule.rule})
			}
		}
	}
	if license.Status == Conflict {
		return TextAttribute{Status: Conflict, Reason: "conflicting_license_types", Evidence: c.evidence}
	}
	return c.scalar()
}
