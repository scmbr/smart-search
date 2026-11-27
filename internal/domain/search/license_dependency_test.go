package search

import "testing"

func TestLicenseTransitionIsNotBasePurchase(t *testing.T) {
	for _, tc := range []struct {
		q, name string
		allowed bool
	}{
		{"сканер уязвимостей", "XSpider Pro лицензия перехода с XSpider на XSpider Pro", false},
		{"сканер уязвимостей", "XSpider Pro основная лицензия", true},
		{"540220", "XSpider Pro лицензия перехода с XSpider на XSpider Pro", true},
	} {
		r, err := Prepare(Request{Query: tc.q, Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		if LicenseDependencyAllowed(r, Document{Name: tc.name}) != tc.allowed {
			t.Fatal(tc)
		}
	}
}
