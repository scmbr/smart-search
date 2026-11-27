package search

import "strings"

// Dependency phrases describe prior-license requirements, not purchased capacity.
func ExcludedLicenseDependencyPhrases(r Request) []string {
	if r.Plan.ExactOnly && r.Plan.LimitReason == "" && len(strings.Fields(r.Query)) == 1 {
		return nil
	}
	return []string{"лицензия перехода", "лицензия на переход", "лицензия миграции"}
}

func LicenseDependencyAllowed(r Request, d Document) bool {
	for _, phrase := range ExcludedLicenseDependencyPhrases(r) {
		if strings.Contains(roleText(d.Name), phrase) {
			return false
		}
	}
	return true
}
