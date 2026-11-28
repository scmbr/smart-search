package search

import (
	"net/url"
	"regexp"
	"strings"
)

type PurposeScope struct{ Group, Vendor string }

var windowsProductHead = regexp.MustCompile(`^(?:(?:новая лицензия|лицензия|купить|продление|обновление|поддержка)\s+)?(?:microsoft\s+)?windows(?:\s|$)`)

// PurposeTaxonomy describes identity evidence already present in catalog fields.
func PurposeTaxonomy(purpose string) (category, identity string) {
	if purpose == "windows_operating_system" {
		return "Операционные системы", "windows"
	}
	return "", ""
}

// PurposeScopes is bounded product evidence, not SKU eligibility or a family map.
func PurposeScopes(purpose string) []PurposeScope {
	switch purpose {
	case "operating_system":
		return []PurposeScope{{"121231", "astralinux"}, {"165644", "redsoft"}, {"166587", "rosa"}}
	case "directory_service":
		return []PurposeScope{{"287015", "astralinux"}, {"387995", "basealt"}, {"408092", "basealt"}}
	}
	return nil
}

func requestedPurpose(r Request) string {
	q := roleText(strings.TrimSpace(r.OriginalQuery))
	if strings.Contains(q, "не ") || strings.Contains(q, "без ") {
		return ""
	}
	if windowsProductHead.MatchString(canonicalQuery(q)) {
		return "windows_operating_system"
	}
	if explicitSoftwareHead.MatchString(q) && strings.Contains(q, "операционная система") {
		return "operating_system"
	}
	if strings.HasPrefix(q, "альтернатива active directory") || strings.HasPrefix(q, "замена active directory") {
		return "directory_service"
	}
	return ""
}

func PurposeAllowed(r Request, d Document) bool {
	p := requestedPurpose(r)
	if p == "" {
		return true
	}
	required, excluded := PurposeLicensePhrases(r)
	name := roleText(d.Name)
	for _, phrase := range excluded {
		if strings.Contains(name, phrase) {
			return false
		}
	}
	if len(required) > 0 {
		found := false
		for _, phrase := range required {
			found = found || strings.Contains(name, phrase)
		}
		if !found {
			return false
		}
	}
	u, err := url.Parse(d.URL)
	if err != nil || u.Scheme != "https" || u.Host != "store.softline.ru" {
		return false
	}
	if category, identity := PurposeTaxonomy(p); identity != "" {
		if d.ProductType != "software" {
			return false
		}
		hasCategory := false
		for _, name := range d.CategoryNames {
			hasCategory = hasCategory || strings.Contains(" "+canonicalQuery(name)+" ", " "+canonicalQuery(category)+" ")
		}
		hasIdentity := false
		for _, word := range queryTokens(d.ProductName) {
			hasIdentity = hasIdentity || word == identity
		}
		return hasCategory && hasIdentity
	}
	for _, s := range PurposeScopes(p) {
		if d.GroupID == s.Group && strings.HasPrefix(u.Path, "/"+s.Vendor+"/") {
			return true
		}
	}
	return false
}

// Phrases constrain only reviewed explicit rights; passing is not full eligibility.
func PurposeLicensePhrases(r Request) (required, excluded []string) {
	switch requestedPurpose(r) {
	case "directory_service":
		return []string{"серверные лицензии", "контроллер домена"}, nil
	case "operating_system":
		q := roleText(r.OriginalQuery)
		if !strings.Contains(q, "образовательн") && !strings.Contains(q, "школ") && !strings.Contains(q, "вуз") {
			return nil, []string{"для образовательных организаций"}
		}
	}
	return nil, nil
}
