package elastic

import (
	"encoding/json"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"strings"
	"testing"
)

func TestNamedFiltersApplyBeforeCollapseInEveryAttempt(t *testing.T) {
	r := d.Request{Query: "Adobe Photoshop", Limit: 4, NamedSelection: &d.NameSelection{CatalogID: "snapshot", Entity: d.NameEntity{Kind: "product", Name: "Adobe Photoshop", Scope: "/adobe/"}, Groups: []string{"group"}}}
	for _, priced := range []bool{true, false} {
		for _, expanded := range []bool{true, false} {
			raw, err := json.Marshal(searchBody(r, priced, expanded))
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{`"GroupID":["group"]`, `"CatalogID":"snapshot"`, `https://store.softline.ru/adobe/`, `"collapse"`} {
				if !strings.Contains(string(raw), required) {
					t.Fatalf("missing %s: %s", required, raw)
				}
			}
		}
	}
}
