package search

import (
	"strings"
	"testing"
)

func TestNamedProductTasksKeepConstraintsAndEvidence(t *testing.T) {
	for _, tc := range []struct {
		query, family, deployment string
		available                 bool
	}{
		{"Астра Линукс операционная система для рабочей станции в наличии", "astra-linux-special-edition", "workstation", true},
		{"операционка Astra Linux для сервера", "astra-linux-special-edition", "server", false},
		{"энидеск удалёнка", "anydesk", "", false},
		{"AnyDesk для удаленного доступа на 2 ПК на год", "anydesk", "", false},
		{"Кибер Бэкап для резервного копирования сервера", "cyber-backup", "server", false},
		{"Кибер Бэкап резервное копирование для рабочей станции", "cyber-backup", "workstation", false},
		{"Сканер-ВС для поиска уязвимостей на 256 IP", "scanner-vs", "", false},
	} {
		r, err := Prepare(Request{Query: tc.query, Limit: 4})
		if err != nil || r.OriginalQuery != tc.query || r.FamilyID != tc.family || r.ProductType != "software" || r.Deployment != tc.deployment || (r.Policy == "available_only") != tc.available || len(r.ProductIntents) != 1 {
			t.Fatalf("%s: %+v %v", tc.query, r, err)
		}
		i := r.ProductIntents[0]
		if i.RuleID == "" || i.Text == "" || !strings.HasPrefix(i.SourceURL, "https://") || i.FamilyID != tc.family {
			t.Fatal("missing task provenance")
		}
		if strings.Contains(tc.query, "2 ПК") && (r.MinDevices != 2 || r.TermMonths != 12) {
			t.Fatal("capacity/term lost")
		}
		if strings.Contains(tc.query, "256 IP") && r.MinIPAddresses != 256 {
			t.Fatal("IP capacity lost")
		}
	}
}

func TestTaskSimplificationNeverGuessesProductOrDropsUnknownMeaning(t *testing.T) {
	for _, q := range []string{"удаленный доступ", "резервное копирование сервера", "операционка", "OtherProduct для проверки уязвимостей"} {
		r, err := Prepare(Request{Query: q, Limit: 4})
		if err != nil || r.FamilyID != "" || len(r.ProductIntents) != 0 {
			t.Fatalf("brand guessed: %+v %v", r, err)
		}
	}
	r, err := Prepare(Request{Query: "AnyDesk удаленка бухгалтерия CODE-5003", Limit: 4})
	if err != nil || !strings.Contains(r.Query, "бухгалтерия") || !strings.Contains(strings.ToLower(r.Query), "code-5003") {
		t.Fatal("unknown requirement or article discarded")
	}
	for _, q := range []string{"AnyDesk не для удаленного доступа", "Кибер Бэкап не для резервного копирования", "Сканер-ВС без проверки уязвимостей", "Кибер Бэкап резервное копирование не для сервера"} {
		r, err := Prepare(Request{Query: q, Limit: 4})
		if err != nil || len(r.UnsupportedConstraints) == 0 {
			t.Fatalf("negated task became positive: %+v %v", r, err)
		}
	}
	for _, r := range []Request{{Query: "AnyDesk удаленка", FamilyID: "cyber-backup", Limit: 4}, {Query: "AnyDesk удаленка", ProductType: "hardware", Limit: 4}} {
		if _, err := Prepare(r); err == nil {
			t.Fatal("conflicting product intent accepted")
		}
	}
}

func TestProductAcronymsAndPhoneticsAreContextBounded(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"касперский смол офис секьюрити", "kaspersky small office security"},
		{"каспер секьюрити для малого офиса", "kaspersky security для small office"},
		{"продление макспатрол епп", "продление maxpatrol endpoint protection platform"},
		{"продление maxpatrlo epp", "продление maxpatrlo endpoint protection platform"},
		{"maxpatrl epp", "maxpatrl endpoint protection platform"},
	} {
		if got := canonicalQuery(tc.q); got != tc.want {
			t.Fatalf("%s: %s", tc.q, got)
		}
	}
	for _, q := range []string{"epp", "registry epp", "ESD-CODE-5003", "секьюрити", "maxpatrol5003 epp"} {
		if got := canonicalQuery(q); strings.Contains(got, "endpoint protection platform") || strings.Contains(got, "security") {
			t.Fatalf("unanchored acronym expanded: %s", got)
		}
	}
	r, err := Prepare(Request{Query: "microsfot office home business", Limit: 4})
	if err != nil || r.Edition != "home_business" {
		t.Fatalf("explicit Office edition lost: %+v %v", r, err)
	}
}
