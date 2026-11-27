package search

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const ProductRolePolicyVersion = "product-role-v2"

type ProductRoleEvidence struct{ Role, RuleID, Source, Text string }

var dependentProductRoles = []string{"library", "addon", "media", "service", "support"}
var roleMedia = regexp.MustCompile(`медиа[- ]комплект|дистрибутив|инсталляционный комплект|установочный комплект`)
var roleSupport = regexp.MustCompile(`\((?:стандартная |расширенная )?(?:техническая поддержка|техподдержка)\)|цена за [0-9]+ техподдержк`)
var roleService = regexp.MustCompile(`one time setup fee|one-time setup fee|(?:^|\s)услуга установки|^сервис (?:непрерывного обследования|установки|по установке)`)
var roleAddon = regexp.MustCompile(`дополнительн[\p{L}]* компонент|дополнительн[\p{L}]* модул|лицензия на расширение|поставщик для интеграции|(?:^|\s)(?:плагин|add-on|plugin)(?:$|[^\p{L}])`)
var roleLibrary = regexp.MustCompile(`(?:^|[^\p{L}\p{N}])(?:sdk|библиотек[\p{L}]*)(?:$|[^\p{L}\p{N}])`)
var roleDeveloperContext = regexp.MustCompile(`(?:^|[^\p{L}])(?:sdk|api|java|javascript)(?:$|[^\p{L}])|\.net|разработ`)
var bundledRole = regexp.MustCompile(`\+|включает|включая|в комплекте`)
var roleSummary = regexp.MustCompile(`тип лицензии:\s*(дистрибутив|расширение лицензии)\s*(?:•|$)`)
var capacityExtension = regexp.MustCompile(`(?:^|[^\p{L}\p{N}])дополнительн[\p{L}]*\s+(?:[0-9]+\s+)?(?:ip(?:[- ]адрес[\p{L}]*)?|уз(?:ел|л[\p{L}]*)|устройств[\p{L}]*)(?:$|[^\p{L}\p{N}])`)
var baseCapacityBundle = regexp.MustCompile(`(?:основная|базовая) лицензия[^;]*\+`)

func roleText(s string) string { return strings.ReplaceAll(strings.ToLower(s), "ё", "е") }

// ClassifyProductRole uses identity/evidence, never a query or relevance label.
func ClassifyProductRole(d Document) ProductRoleEvidence {
	identities := []struct{ source, text string }{{"title", d.Name}, {"product_name", d.ProductName}}
	makeEvidence := func(role, id, source, text string) ProductRoleEvidence {
		return ProductRoleEvidence{Role: role, RuleID: id, Source: source, Text: text}
	}
	if match := roleSummary.FindStringSubmatch(roleText(d.Description)); len(match) > 0 {
		role := "media"
		if match[1] == "расширение лицензии" {
			role = "addon"
		}
		return makeEvidence(role, "feed-license-type-v1", "description_license_type", match[1])
	}
	for _, identity := range identities {
		if roleMedia.MatchString(roleText(identity.text)) && !bundledRole.MatchString(roleText(identity.text)) {
			return makeEvidence("media", "identity-media-v1", identity.source, identity.text)
		}
	}
	if !baseCapacityBundle.MatchString(roleText(d.Name)) {
		for _, identity := range identities {
			text := roleText(identity.text)
			if capacityExtension.MatchString(text) && !baseCapacityBundle.MatchString(text) {
				return makeEvidence("addon", "identity-capacity-extension-v1", identity.source, identity.text)
			}
		}
	}
	for _, rule := range []struct {
		role, id string
		pattern  *regexp.Regexp
	}{
		{"support", "identity-support-v1", roleSupport}, {"service", "identity-service-v1", roleService}, {"addon", "identity-addon-v1", roleAddon},
	} {
		for _, identity := range identities {
			if rule.pattern.MatchString(roleText(identity.text)) {
				return makeEvidence(rule.role, rule.id, identity.source, identity.text)
			}
		}
	}
	if d.ProductType != "hardware" {
		for _, identity := range identities {
			if roleLibrary.MatchString(roleText(identity.text)) && roleDeveloperContext.MatchString(roleText(identity.text)) {
				return makeEvidence("library", "identity-library-v1", identity.source, identity.text)
			}
		}
		if len(d.CategoryNames) > 0 {
			category := roleText(d.CategoryNames[0])
			if (strings.Contains(category, "библиотек") && strings.Contains(category, "разработ")) || strings.Contains(category, "компоненты для разработчиков") {
				return makeEvidence("library", "leaf-library-v1", "leaf_category", d.CategoryNames[0])
			}
			if category == "техническая поддержка программного обеспечения" {
				return makeEvidence("support", "leaf-support-v1", "leaf_category", d.CategoryNames[0])
			}
		}
		u, err := url.Parse(d.URL)
		if err == nil && u.Scheme == "https" && u.Host == "store.softline.ru" {
			vendor := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")[0]
			key := vendor + "/" + d.GroupID
			switch key {
			case "northwoodssoftware/208224":
				return makeEvidence("library", "vendor-godiagram-v1", "https://godiagram.com/winforms/learn/", key)
			case "aspose-pty-ltd/174704":
				return makeEvidence("library", "vendor-aspose-diagram-v1", "https://docs.aspose.com/diagram/net/", key)
			}
		}
	}
	return ProductRoleEvidence{}
}

var requestedRolePatterns = []struct {
	role    string
	pattern *regexp.Regexp
}{
	{"addon", capacityExtension},
	{"library", regexp.MustCompile(`(?:^|[^\p{L}\p{N}])(?:sdk|библиотека|библиотеку|библиотеки)(?:$|[^\p{L}\p{N}])`)},
	{"addon", regexp.MustCompile(`дополнительн[\p{L}]* компонент|(?:^|\s)(?:дополнени[\p{L}]*|плагин[\p{L}]*|расширение|add-on|plugin)(?:$|[^\p{L}])|поставщик для интеграции`)},
	{"media", roleMedia},
	{"service", regexp.MustCompile(`(?:^|\s)(?:услуга|сервис установки|setup fee)(?:$|[^\p{L}])`)},
}
var explicitSoftwareHead = regexp.MustCompile(`^(?:(?:мне )?нужн[\p{L}]* |ищу |хочу найти )?(?:(?:защищенн[\p{L}]*|российск[\p{L}]*|отечественн[\p{L}]*|корпоративн[\p{L}]*) )*(?:операционная система|офисные приложения|программное обеспечение|программа)(?:$|\s)`)

func prepareProductRole(r *Request) error {
	r.RequestedProductRole = ""
	r.NamedLibrary = false
	if r.Plan.ExactOnly && r.Plan.LimitReason == "" && len(strings.Fields(r.Query)) == 1 {
		return nil
	}
	query := roleText(r.OriginalQuery)
	// These identities refer only to the two source-reviewed libraries above.
	// Keep named product/vendor searches usable without changing general task ranking.
	switch strings.Join(strings.Fields(query), " ") {
	case "godiagram", "northwoods", "northwoods software", "aspose", "aspose.diagram", "aspose diagram":
		r.NamedLibrary = true
	}
	for _, rule := range requestedRolePatterns {
		matches := rule.pattern.FindAllStringIndex(query, -1)
		positive := false
		for _, match := range matches {
			prefix := strings.Fields(query[:match[0]])
			// The boundary may consume a space; trim before checking the preceding word.
			if len(prefix) > 0 && slices.Contains([]string{"не", "без", "вместо"}, prefix[len(prefix)-1]) {
				continue
			}
			if rule.role == "library" && len(prefix) > 0 && prefix[len(prefix)-1] == "для" {
				continue
			}
			positive = true
		}
		if positive {
			if r.RequestedProductRole != "" && r.RequestedProductRole != rule.role {
				return ErrInvalidQuery
			}
			r.RequestedProductRole = rule.role
		}
	}
	if r.ProductType == "" && explicitSoftwareHead.MatchString(query) {
		r.ProductType = "software"
	}
	return nil
}

func ProductRoleAllowed(r Request, role string) bool {
	if r.RequestedProductRole != "" {
		return role == r.RequestedProductRole
	}
	if role == "library" && r.NamedLibrary {
		return true
	}
	if r.Plan.ExactOnly && r.Plan.LimitReason == "" && len(strings.Fields(r.Query)) == 1 {
		return true
	}
	if role == "support" && r.Kind == "support" {
		return true
	}
	return !slices.Contains(dependentProductRoles, role)
}

func ExcludedProductRoles(r Request) []string {
	var out []string
	for _, role := range dependentProductRoles {
		if !ProductRoleAllowed(r, role) {
			out = append(out, role)
		}
	}
	return out
}

func SetProductRole(d *Document) {
	e := ClassifyProductRole(*d)
	d.ProductRole = e.Role
	d.ProductRoleRule = e.RuleID
	d.ProductRoleSource = e.Source
	d.ProductRoleText = e.Text
}

func ValidProductRole(d Document) bool {
	e := ClassifyProductRole(d)
	return d.ProductRole == e.Role && d.ProductRoleRule == e.RuleID && d.ProductRoleSource == e.Source && d.ProductRoleText == e.Text
}
