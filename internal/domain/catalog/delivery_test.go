package catalog

import "testing"

func TestDeliveryDoesNotContradictLicenseRole(t *testing.T) {
	for _, tc := range []struct{ license, want string }{{"Full", "new_license"}, {"Продление", "renewal"}, {"Техподдержка", "support"}, {"", ""}} {
		ps := []Parameter{param("Тип лицензии", "ESD")}
		if tc.license != "" {
			ps = append(ps, param("Тип лицензии", tc.license))
		}
		c := Normalize(Offer{Parameters: ps})
		if c.OfferKind.Value != tc.want || c.OfferKind.Status == Conflict || c.DeliveryMethod.Status != Known || c.DeliveryMethod.Value != "esd" || len(c.DeliveryMethod.Evidence) != 1 {
			t.Fatalf("%s: %+v", tc.license, c)
		}
		if tc.license == "" && c.LicenseType.Status != Unknown {
			t.Fatal("delivery inferred license type")
		}
	}
	c := Normalize(Offer{Parameters: []Parameter{param("Тип лицензии", "Full"), param("Тип лицензии", "ESD"), param("Тип лицензии", "Продление")}})
	if c.OfferKind.Status != Conflict {
		t.Fatal("contradictory roles hidden by delivery")
	}
	c = Normalize(Offer{Parameters: []Parameter{param("Тип лицензии", "Full"), param("Тип лицензии", "ESD"), param("Тип лицензии", "unrecognized")}})
	if c.OfferKind.Status == Known {
		t.Fatal("unknown type ignored")
	}
}

func TestMicrosoft365EditionComesFromSameSKU(t *testing.T) {
	for _, tc := range []struct{ title, edition string }{{"Microsoft 365 Персональный, Электронная подписка для 1 пользователя", "personal"}, {"Microsoft 365 Personal", "personal"}, {"Microsoft 365 Family", "family"}, {"Microsoft 365 для семьи, на 6 пользователей", "family"}, {"Other 365 Personal", ""}} {
		c := Normalize(Offer{Name: tc.title})
		if c.Edition.Value != tc.edition || c.Version.Status != Unknown {
			t.Fatalf("%s: %+v", tc.title, c)
		}
	}
}
