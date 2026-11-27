package catalog

import "testing"

func TestTypedCapacityAndDeploymentEvidence(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		devices, ips, participants int
		deployment                 string
	}{
		{"Microsoft Office для дома 2024, Электронная лицензия на 1 ПК, бессрочная", 1, 0, 0, ""},
		{"СКАНЕР-ВС 7, Основная лицензия Enterprise на 1 год, на 256 IP активов", 0, 256, 0, ""},
		{"МТС Линк Optimum 30 (лицензия в месяц при покупке на год, до 30 участников вебинара)", 0, 0, 30, ""},
		{"КриптоПро CSP 5.0: серверная лицензия СКЗИ", 0, 0, 0, "server"},
		{"КриптоПро CSP 5.0, на одном сервере", 0, 0, 0, "server"},
		{"КриптоПро CSP 5.0, на одном рабочем месте", 0, 0, 0, "workstation"},
		{"Product на 3 пользователей", 0, 0, 0, ""},
	} {
		c := Normalize(Offer{Name: tc.name})
		if c.DevicesLimit.Value != tc.devices || c.IPAddressesLimit.Value != tc.ips || c.ParticipantsLimit.Value != tc.participants || c.Deployment.Value != tc.deployment {
			t.Fatalf("%s: %+v", tc.name, c)
		}
		for _, a := range []IntegerAttribute{c.DevicesLimit, c.IPAddressesLimit, c.ParticipantsLimit} {
			if a.Value > 0 && (a.Status != Known || len(a.Evidence) != 1 || a.Evidence[0].Source != "title") {
				t.Fatal("missing capacity evidence")
			}
		}
		if c.TermKind.Status != Unknown {
			t.Fatal("billing/title term became license term")
		}
	}
	for _, title := range []string{"Product от 5 до 10 ПК", "Product на 5-10 ПК", "Product не на 5 ПК", "Product на -5 ПК", "Product на 1000001 ПК", "Product на 5 ПК/пользователей", "Product на 5 ПК / пользователей", "Product на 5 ПКabc", "Product цена за 5 ПК", "Product на 5 ПК на 1000000000 ПК"} {
		if c := Normalize(Offer{Name: title}); c.DevicesLimit.Status != Unknown {
			t.Fatalf("ambiguous capacity: %s %+v", title, c.DevicesLimit)
		}
	}
	c := Normalize(Offer{Name: "Product на 5 ПК на 10 ПК"})
	if c.DevicesLimit.Status != Conflict {
		t.Fatal("contradictory title counts hidden")
	}
	c = Normalize(Offer{Name: "Product", Description: "на 5 ПК до 30 участников на 256 IP", Parameters: []Parameter{{Name: "К-во узлов", Value: "256", Known: true}}})
	if c.DevicesLimit.Status != Unknown || c.IPAddressesLimit.Status != Unknown || c.ParticipantsLimit.Status != Unknown {
		t.Fatal("description/nodes became typed capacity")
	}
}

func TestOfficeHome2024AliasIsBounded(t *testing.T) {
	for _, title := range []string{"Microsoft Office для дома 2024, лицензия на 1 ПК", "Microsoft Office Home 2024"} {
		c := Normalize(Offer{Name: title})
		if c.Edition.Value != "home_student" || c.Version.Value != "2024" {
			t.Fatalf("Home 2024 alias missing: %+v", c)
		}
	}
	for _, title := range []string{"Other для дома 2024", "Microsoft Office для дома 2021", "Microsoft Office Home 2024beta"} {
		if c := Normalize(Offer{Name: title}); c.Edition.Status != Unknown {
			t.Fatalf("alias escaped product/version: %s", title)
		}
	}
	if c := Normalize(Offer{Name: "Microsoft Office для дома и бизнеса 2024"}); c.Edition.Value != "home_business" {
		t.Fatal("business edition changed")
	}
}

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
