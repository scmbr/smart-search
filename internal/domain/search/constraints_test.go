package search

import "testing"

func TestParseExplicitQueryConstraints(t *testing.T) {
	for _, tc := range []struct {
		query, edition, term, deployment string
		users, months                    int
		available                        bool
	}{
		{"AnyDesk Standard на год", "standard", "fixed", "", 0, 12, false},
		{"Продукт на шесть пользователей на 2 года", "", "fixed", "", 6, 24, false},
		{"Продукт для шести пользователей на один год", "", "fixed", "", 6, 12, false},
		{"Product бессрочно для рабочей станции", "", "perpetual", "workstation", 0, 0, false},
		{"Кибер Бэкап расширенный для сервера в наличии", "extended", "", "server", 0, 0, true},
	} {
		r, err := Prepare(Request{Query: tc.query, Limit: 4})
		if err != nil || r.Edition != tc.edition || r.TermKind != tc.term || r.TermMonths != tc.months || r.MinUsers != tc.users || r.Deployment != tc.deployment || (r.Policy == "available_only") != tc.available || r.OriginalQuery != tc.query {
			t.Fatalf("%s: %+v err=%v", tc.query, r, err)
		}
		if len(r.Constraints) == 0 {
			t.Fatal("missing parsed constraint evidence")
		}
	}
}

func TestConstraintsCannotSilentlyChangeUnitsOrMeaning(t *testing.T) {
	raw, err := Prepare(Request{Query: "  ПО  для  сервера  ", Limit: 4})
	if err != nil || raw.ProductType != "software" || raw.OriginalQuery != "  ПО  для  сервера  " {
		t.Fatal("whitespace changed software intent/input")
	}
	for _, q := range []string{"Product на 0 пользователей", "Product на -5 пользователей", "Product на 12 месяцев на 2 года", "Product редакция Standard редакция Enterprise", "Product от 5 до 10 пользователей"} {
		if _, err := Prepare(Request{Query: q, Limit: 4}); err == nil {
			t.Fatalf("conflict/invalid accepted: %s", q)
		}
	}
	if _, err := Prepare(Request{Query: "Product на год", Limit: 4, TermMonths: 24}); err == nil {
		t.Fatal("CLI/text term conflict accepted")
	}
	r, err := Prepare(Request{Query: "Product на 2 ПК", Limit: 4})
	if err != nil || r.MinUsers != 0 || r.MinDevices != 2 || len(r.UnsupportedConstraints) != 0 {
		t.Fatalf("device capacity guessed as users: %+v %v", r, err)
	}
	r, err = Prepare(Request{Query: "Product не в наличии", Limit: 4})
	if err != nil || len(r.UnsupportedConstraints) == 0 {
		t.Fatal("negated availability silently became available_only")
	}
	r, err = Prepare(Request{Query: "AnyDesk не Standard", Limit: 4})
	if err != nil || len(r.UnsupportedConstraints) == 0 {
		t.Fatal("negative edition treated as positive")
	}
}

func TestTypedCapacityQueries(t *testing.T) {
	for _, q := range []string{"Product не на 5 ПК", "Product на 5 ПК/пользователей"} {
		r, err := Prepare(Request{Query: q, Limit: 4})
		if err != nil || len(r.UnsupportedConstraints) == 0 {
			t.Fatalf("ambiguous units/negation: %s %+v %v", q, r, err)
		}
	}
	for _, q := range []string{"Microsoft Office Home 2024", "Microsoft Office для дома 2024"} {
		r, err := Prepare(Request{Query: q, Limit: 4})
		if err != nil || r.Version != "2024" || r.Edition != "home_student" {
			t.Fatalf("Office rename not parsed: %+v %v", r, err)
		}
	}
	r, err := Prepare(Request{Query: "Product на 2 ПК на 256 IP на 30 участников", Limit: 4})
	if err != nil || r.MinDevices != 2 || r.MinIPAddresses != 256 || r.MinParticipants != 30 || r.MinUsers != 0 || len(r.UnsupportedConstraints) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	for _, q := range []string{"Product на 0 ПК", "Product на -1 IP", "Product на 1000001 участников", "Product на 2 ПК на 3 ПК", "Product от 5 до 10 участников"} {
		if _, err := Prepare(Request{Query: q, Limit: 4}); err == nil {
			t.Fatalf("invalid capacity accepted: %s", q)
		}
	}
	if _, err := Prepare(Request{Query: "Product на 2 ПК", Limit: 4, MinDevices: 3}); err == nil {
		t.Fatal("CLI/text conflict accepted")
	}
	if _, err := Prepare(Request{Query: "Product", Limit: 4, MinIPAddresses: -1}); err == nil {
		t.Fatal("negative CLI capacity accepted")
	}
}
