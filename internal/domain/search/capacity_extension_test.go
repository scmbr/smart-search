package search

import "testing"

func TestCapacityExtensionIdentityAndQuery(t *testing.T) {
	for _, tc := range []struct{ name, description, role string }{
		{"XSpider Pro Pentest + Audit на дополнительный IP, к лицензии на 4 IP", "", "addon"},
		{"Продукт на 3 дополнительных устройства", "", "addon"},
		{"Продукт на дополнительные 10 узлов", "", "addon"},
		{"Продукт на дополнительный IP-адрес", "", "addon"},
		{"Продукт основная лицензия на 256 IP", "", ""},
		{"Продукт основная лицензия + дополнительные 10 узлов", "", ""},
		{"Продукт", "Поддерживает дополнительные IP и узлы", ""},
		{"Дополнительная сертификация устройств", "", ""},
	} {
		if e := ClassifyProductRole(Document{Name: tc.name, Description: tc.description}); e.Role != tc.role {
			t.Fatalf("%s: %+v", tc.name, e)
		}
	}
	for _, tc := range []struct {
		q, role     string
		unsupported bool
	}{
		{"XSpider дополнительный IP", "addon", false},
		{"дополнительный узел", "addon", false},
		{"без дополнительного IP", "", false},
		{"на 256 IP", "", false},
		{"2 дополнительных IP", "addon", true},
		{"два дополнительных IP", "addon", true},
		{"дополнительный IP к лицензии на 4 IP", "addon", true},
		{"лицензия на 10 узлов", "", true},
	} {
		r, err := Prepare(Request{Query: tc.q, Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		if r.RequestedProductRole != tc.role || (len(r.UnsupportedConstraints) > 0) != tc.unsupported {
			t.Fatalf("%s: %+v", tc.q, r)
		}
		if tc.unsupported && r.MinIPAddresses != 0 {
			t.Fatal("base quantity promoted to purchase capacity", r.MinIPAddresses)
		}
	}
	if e := ClassifyProductRole(Document{Name: "Продукт основная лицензия + дополнительные 10 узлов", ProductName: "Дополнительные узлы"}); e.Role != "" {
		t.Fatalf("bundle inferred from group: %+v", e)
	}
}
