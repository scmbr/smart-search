package search

import "testing"

func TestPurposeUsesOriginalRequestAndReviewedScope(t *testing.T) {
	for _, tc := range []struct{ query, purpose string }{
		{"защищенная российская операционная система для компьютеров организации", "operating_system"},
		{"альтернатива Active Directory для управления пользователями", "directory_service"},
		{"управление операционной системой", ""}, {"управление Active Directory", ""},
		{"не нужна альтернатива Active Directory", ""},
	} {
		r, err := Prepare(Request{Query: tc.query, Limit: 4})
		if err != nil || r.Plan.Purpose != tc.purpose {
			t.Fatalf("%s: %+v %v", tc.query, r.Plan, err)
		}
	}
	r, _ := Prepare(Request{Query: "защищенная российская операционная система", Limit: 4})
	for _, tc := range []struct {
		group, url string
		allowed    bool
	}{
		{"165644", "https://store.softline.ru/redsoft/fixture/", true},
		{"360395", "https://store.softline.ru/redsoft/fixture/", false},
		{"165644", "https://store.softline.ru/wrong/fixture/", false},
		{"unknown", "https://store.softline.ru/redsoft/fixture/", false},
	} {
		if PurposeAllowed(r, Document{GroupID: tc.group, URL: tc.url}) != tc.allowed {
			t.Fatal(tc)
		}
	}
}

func TestPurposeLicenseConditions(t *testing.T) {
	for _, tc := range []struct {
		q, group, vendor, name string
		allowed                bool
	}{
		{"операционная система", "165644", "redsoft", "РЕД ОС для образовательных организаций", false},
		{"операционная система для школы", "165644", "redsoft", "РЕД ОС для образовательных организаций", true},
		{"альтернатива Active Directory", "287015", "astralinux", "ALD Pro клиентская лицензия", false},
		{"альтернатива Active Directory", "287015", "astralinux", "ALD Pro серверные лицензии", true},
		{"альтернатива Active Directory", "387995", "basealt", "Альт Домен на 1 контроллер домена", true},
		{"альтернатива Active Directory", "287015", "astralinux", "ALD Pro", false},
	} {
		r, err := Prepare(Request{Query: tc.q, Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		if PurposeAllowed(r, Document{GroupID: tc.group, URL: "https://store.softline.ru/" + tc.vendor + "/fixture/", Name: tc.name}) != tc.allowed {
			t.Fatal(tc)
		}
	}
}
