package catalog

import "testing"

func TestOfferTermsUseExplicitLicenseParameterOnly(t *testing.T) {
	for _, tc := range []struct {
		value, kind string
		months      int
	}{
		{"12 мес.", "fixed", 12}, {"2 года", "fixed", 24}, {"бессрочная", "perpetual", 0},
		{"Не определено", "", 0}, {"12-24 мес.", "", 0}, {"до 2027 года", "", 0},
	} {
		c := Normalize(Offer{Name: "Product", Parameters: []Parameter{{Name: "Срок действия", Value: tc.value, RawValue: tc.value, Known: true}}})
		if c.TermKind.Value != tc.kind || c.TermMonths.Value != tc.months {
			t.Fatalf("%s: %+v %+v", tc.value, c.TermKind, c.TermMonths)
		}
		if tc.kind != "" && (c.TermKind.Status != Known || len(c.TermKind.Evidence) == 0) {
			t.Fatal("missing term provenance")
		}
	}
	c := Normalize(Offer{Name: "Product на 1 год", Parameters: []Parameter{{Name: "Срок техподдержки", Value: "12 мес.", Known: true}}})
	if c.TermKind.Status != Unknown {
		t.Fatal("support/title term treated as license term")
	}
	c = Normalize(Offer{Parameters: []Parameter{{Name: "Срок действия", Value: "12 мес.", Known: true}, {Name: "Срок действия", Value: "бессрочная", Known: true}}})
	if c.TermKind.Status != Conflict {
		t.Fatal("conflicting fixed/perpetual terms hidden")
	}
}

func TestSKUConstraintEvidenceNeverComesFromAnotherSKU(t *testing.T) {
	office := Normalize(Offer{Name: "Microsoft Office для дома и бизнеса 2024"})
	if office.Version.Value != "2024" || office.Edition.Value != "home_business" {
		t.Fatal("explicit Office attributes missing")
	}
	m365 := Normalize(Offer{Name: "Microsoft 365 на 6 пользователей"})
	if m365.Version.Status != Unknown || m365.UsersLimit.Value != 6 {
		t.Fatal("365 treated as version or explicit user count lost")
	}
	c := Normalize(Offer{Name: "AnyDesk (лицензия на 1 год), Standard"})
	if c.Edition.Status != Known || c.Edition.Value != "standard" || c.TermKind.Status != Unknown {
		t.Fatalf("wrong edition/guessed term: %+v", c)
	}
	c = Normalize(Offer{Name: "Кибер Бэкап Расширенная редакция, для физического сервера"})
	if c.Edition.Value != "extended" || c.Deployment.Value != "server" {
		t.Fatalf("explicit attributes lost: %+v", c)
	}
	c = Normalize(Offer{Name: "Microsoft 365 подписка на 6 пользователей", Parameters: []Parameter{{Name: "К-во пользователей до", Value: "5", Known: true}}})
	if c.UsersLimit.Status != Conflict {
		t.Fatal("title/parameter capacity conflict ignored")
	}
	c = Normalize(Offer{Name: "Product на 6 ПК"})
	if c.UsersLimit.Status != Unknown {
		t.Fatal("devices interpreted as users")
	}
	c = Normalize(Offer{Name: "Product на 6 пользователей на 10 пользователей"})
	if c.UsersLimit.Status != Conflict {
		t.Fatal("multiple title capacities collapsed")
	}
	c = Normalize(Offer{Name: "Product от 5 до 10 пользователей"})
	if c.UsersLimit.Status != Unknown {
		t.Fatal("range treated as a fixed user count")
	}
	c = Normalize(Offer{Name: "Product", Description: "для сервера", Parameters: []Parameter{{Name: "Редакция", Value: "Enterprise", Known: true}}})
	if c.Edition.Value != "enterprise" || c.Deployment.Status != Unknown {
		t.Fatal("description used as license evidence")
	}
}
