package catalog

import (
	"reflect"
	"testing"
)

func param(name, value string) Parameter {
	return Parameter{Name: name, RawValue: value, Value: value, Known: true}
}

func TestOfferRoles(t *testing.T) {
	for _, tc := range []struct {
		name, license, title, want string
		status                     Knowledge
	}{
		{"full", "Полная версия", "Product", "new_license", Known},
		{"full english", "Full", "Product", "new_license", Known},
		{"renewal", "Продление", "Product", "renewal", Known},
		{"upgrade", "Обновление", "Product", "upgrade", Known},
		{"crossgrade", "Перекрестное обновление", "Product", "upgrade", Known},
		{"support", "Техподдержка", "Product", "support", Known},
		{"subscription", "Подписка", "Product", "", Unknown},
		{"subscription renewal", "Подписка", "Product — продление подписки", "renewal", Known},
		{"module", "Модуль", "Product", "", Unknown},
		{"base", "Базовая", "Product", "", Unknown},
		{"addon", "Add-on", "Product", "", Unknown},
		{"conflict", "Полная версия", "Product — продление", "", Conflict},
		{"support certificate", "", "Astra Linux — сертификат технической поддержки", "support", Known},
		{"bundled support", "Полная версия", "Новая лицензия с технической поддержкой", "new_license", Known},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Offer{Name: tc.title, Parameters: []Parameter{param("Тип продукта", "софт")}}
			if tc.license != "" {
				o.Parameters = append(o.Parameters, param("Тип лицензии", tc.license))
			}
			got := Normalize(o).OfferKind
			if got.Status != tc.status || got.Value != tc.want {
				t.Fatalf("role=%+v", got)
			}
		})
	}
}

func TestVersionAndEditionAreDifferent(t *testing.T) {
	for _, tc := range []struct {
		name, field, title, version, edition string
		status                               Knowledge
	}{
		{"numeric", "5.0", "КриптоПро CSP 5.0", "5.0", "", Known},
		{"title fallback", "другое или отсутствует", "КриптоПро CSP 5.0 на 10 пользователей, 12 мес.", "5.0", "", Known},
		{"edition", "Standard", "Product 10 users 2026", "", "standard", Unknown},
		{"edition plus version", "Смоленск", "Astra Linux Special Edition 1.7", "1.7", "smolensk", Known},
		{"contradiction", "4.0", "КриптоПро CSP 5.0", "", "", Conflict},
		{"not arbitrary numbers", "", "Product 5 users, 2026, SKU-5.0", "", "", Unknown},
		{"adjacent versions", "", "КриптоПро CSP 4.0,КриптоПро CSP 5.0", "", "", Conflict},
		{"version suffix", "", "КриптоПро CSP 5.0beta", "", "", Unknown},
		{"version prefix", "", "неКриптоПро CSP 5.0", "", "", Unknown},
		{"upgrade source version", "", "Обновление с КриптоПро CSP 4.0 на 5.0", "", "", Unknown},
		{"upgrade target not proven", "", "Upgrade КриптоПро CSP 5.0", "", "", Unknown},
		{"explicit upgrade version", "5.0", "Обновление с КриптоПро CSP 4.0 на 5.0", "5.0", "", Known},
		{"no version from 365", "", "Microsoft 365 Family 12 месяцев", "", "", Unknown},
		{"unsupported version", "beta release", "Product", "", "", Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Offer{Name: tc.title, Parameters: []Parameter{param("Тип продукта", "софт")}}
			if tc.field != "" {
				o.Parameters = append(o.Parameters, param("Версия", tc.field))
			}
			got := Normalize(o)
			if got.Version.Status != tc.status || got.Version.Value != tc.version || got.Edition.Value != tc.edition {
				t.Fatalf("got=%+v", got)
			}
		})
	}
}

func TestPlatformsAndArchitecture(t *testing.T) {
	o := Offer{Parameters: []Parameter{param("Совместимая архитектура/платформа", "x86-64"), param("Операционная система", "Windows 11 Pro")}}
	got := Normalize(o)
	if got.Platforms.Status != Unknown || !reflect.DeepEqual(got.Architectures.Values, []string{"x86_64"}) {
		t.Fatalf("architecture leaked into OS: %+v", got)
	}
	o.Parameters = append(o.Parameters, param("Платформа", "Windows; Linux"), param("Платформа", "Linux"))
	got = Normalize(o)
	if got.Platforms.Status != Known || !reflect.DeepEqual(got.Platforms.Values, []string{"linux", "windows"}) {
		t.Fatalf("platforms=%+v", got.Platforms)
	}
	o.Parameters = append(o.Parameters, param("Платформа", "UnknownOS"))
	if got := Normalize(o).Platforms; got.Status != Unknown || len(got.Values) != 0 {
		t.Fatalf("partial set trusted: %+v", got)
	}
	o.Parameters = []Parameter{param("Платформа", "Windows"), param("Платформа", ";;;")}
	if got := Normalize(o).Platforms; got.Status != Unknown {
		t.Fatalf("empty malformed set accepted: %+v", got)
	}
}

func TestStrictCapacityAndUnits(t *testing.T) {
	for _, tc := range []struct {
		raw, unit string
		want      int
	}{
		{"5", "", 5}, {"5", "пользователей", 5}, {"1000000", "users", 1000000},
		{"000", "", 0}, {"0", "", 0}, {"-1", "", 0}, {"1-10", "", 0}, {"1.5", "", 0}, {"1000001", "", 0}, {"5", "nodes", 0}, {"unlimited", "", 0},
	} {
		t.Run(tc.raw+tc.unit, func(t *testing.T) {
			p := param("К-во пользователей до", tc.raw)
			p.Unit = tc.unit
			got := Normalize(Offer{Parameters: []Parameter{p, param("К-во узлов", "2")}})
			if got.UsersLimit.Value != tc.want || (got.UsersLimit.Status == Known) != (tc.want > 0) || got.Nodes.Value != 2 {
				t.Fatalf("got=%+v", got)
			}
		})
	}
	o := Offer{Parameters: []Parameter{param("К-во пользователей до", "5"), param("К-во пользователей до", "10")}}
	if got := Normalize(o).UsersLimit; got.Status != Conflict || got.Value != 0 {
		t.Fatalf("conflict=%+v", got)
	}
}

func TestEvidenceUnknownAndNoMutation(t *testing.T) {
	o := Offer{GroupID: "shared", Name: "Product", Description: "Новая лицензия. Windows 5.0", Parameters: []Parameter{
		param("Тип лицензии", "Полная версия"), param("Тип лицензии", " full "),
		{Name: "Версия", RawValue: "5.0", Value: "5.0", Known: false},
		param("Платформа", "не определено"),
	}}
	before := append([]Parameter(nil), o.Parameters...)
	got := Normalize(o)
	if got.PolicyVersion != NormalizationVersion || got.OfferKind.Value != "new_license" || got.Version.Status != Unknown || got.Platforms.Status != Unknown {
		t.Fatalf("got=%+v", got)
	}
	if len(got.OfferKind.Evidence) != 2 || got.OfferKind.Evidence[0].ParameterIndex != 0 || got.OfferKind.Evidence[0].RawValue != "Полная версия" || got.OfferKind.Evidence[0].RuleID == "" {
		t.Fatalf("evidence=%+v", got.OfferKind.Evidence)
	}
	if !reflect.DeepEqual(o.Parameters, before) {
		t.Fatal("mutated input")
	}
	other := Normalize(Offer{GroupID: "shared", Name: "Product"})
	if other.OfferKind.Status != Unknown || other.Version.Status != Unknown {
		t.Fatal("inherited another SKU")
	}
	if !reflect.DeepEqual(got, Normalize(o)) {
		t.Fatal("non-deterministic normalization")
	}
}

func TestScalarConflictsAndHardware(t *testing.T) {
	for _, params := range [][]Parameter{
		{param("Тип лицензии", "Full"), param("Тип лицензии", "Продление")},
		{param("Тип лицензии", "Full"), param("Тип лицензии", "unrecognized")},
	} {
		got := Normalize(Offer{Parameters: params})
		if got.OfferKind.Value != "" || got.OfferKind.Status == Known {
			t.Fatalf("ambiguous role trusted: %+v", got)
		}
	}
	got := Normalize(Offer{Name: "Новая лицензия КриптоПро CSP 5.0", Parameters: []Parameter{param("Тип продукта", "электроника")}})
	if got.OfferKind.Status != Unknown || got.Version.Status != Unknown {
		t.Fatalf("hardware name used as software: %+v", got)
	}
}
