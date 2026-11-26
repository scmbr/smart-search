package catalog

import "testing"

func TestValidationEligibility(t *testing.T) {
	valid := Validation{Complete: true, Categories: 2, Offers: 3, Accepted: 3}
	if !valid.Eligible() {
		t.Fatal("valid catalog rejected")
	}
	for name, change := range map[string]func(*Validation){
		"incomplete":    func(v *Validation) { v.Complete = false },
		"empty":         func(v *Validation) { v.Accepted = 0; v.Offers = 0 },
		"no categories": func(v *Validation) { v.Categories = 0 },
		"issues":        func(v *Validation) { v.Issues = 1 },
		"rejected":      func(v *Validation) { v.Rejected = 1 },
		"inconsistent":  func(v *Validation) { v.Offers++ },
	} {
		t.Run(name, func(t *testing.T) {
			v := valid
			change(&v)
			if v.Eligible() {
				t.Fatal("invalid catalog eligible")
			}
		})
	}
}

func TestIdentity(t *testing.T) {
	a := Identity{SourceSHA256: "abc", SchemaVersion: "reader-v1"}
	if a.ID() != a.ID() || len(a.ID()) != 64 {
		t.Fatal("unstable ID")
	}
	b := a
	b.SchemaVersion = "reader-v2"
	c := a
	c.SourceSHA256 = "def"
	if a.ID() == b.ID() || a.ID() == c.ID() {
		t.Fatal("identity collision")
	}
}
