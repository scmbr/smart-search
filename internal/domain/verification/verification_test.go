package verification

import (
	"testing"
	"time"
)

// Fixtures describe synthetic compatibility evidence, not Softline products.
func fixture() Input {
	fact := func(offer string, values ...string) Fact {
		return Fact{Status: Known, Values: values, Source: "synthetic:structured", Confirmed: true, OfferID: offer, Catalog: "snapshot"}
	}
	a := map[string]Fact{"platform": fact("base", "windows"), "version": fact("base", "11"), "users": fact("base")}
	b := map[string]Fact{"platforms": fact("addon", "linux", "windows"), "version": fact("addon", "11"), "users": fact("addon"), "purchase": fact("addon", "renewal")}
	a["users"] = Fact{Status: Known, Number: 5, NumberKnown: true, Unit: "users", Source: "synthetic:structured", Confirmed: true, OfferID: "base", Catalog: "snapshot"}
	b["users"] = Fact{Status: Known, Number: 10, NumberKnown: true, Unit: "users", Source: "synthetic:structured", Confirmed: true, OfferID: "addon", Catalog: "snapshot"}
	return Input{AnchorID: "base", RecommendedID: "addon", Catalog: "snapshot", Policy: Policy{ID: "synthetic-v1", Rules: []Rule{
		{ID: "platform", Operator: Contains, AnchorField: "platform", RecommendedField: "platforms"},
		{ID: "version", Operator: Equals, AnchorField: "version", RecommendedField: "version"},
		{ID: "capacity", Operator: AtLeast, AnchorField: "users", RecommendedField: "users"},
		{ID: "rights", Operator: LicenseEligible, RecommendedField: "purchase"},
	}}, Anchor: a, Recommended: b, Context: Context{Rights: map[string]Fact{"renewal": fact("base", "eligible")}}}
}

func TestMandatoryChecks(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Input)
		want   Status
	}{
		{"complete", func(*Input) {}, Passed},
		{"version mismatch", func(i *Input) { f := i.Recommended["version"]; f.Values = []string{"10"}; i.Recommended["version"] = f }, Failed},
		{"missing platform", func(i *Input) { delete(i.Anchor, "platform") }, Unknown},
		{"capacity exceeded", func(i *Input) { f := i.Anchor["users"]; f.Number = 11; i.Anchor["users"] = f }, Failed},
		{"unit mismatch", func(i *Input) { f := i.Anchor["users"]; f.Unit = "devices"; i.Anchor["users"] = f }, Unknown},
		{"missing renewal rights", func(i *Input) { i.Context = Context{} }, Unknown},
		{"denied rights", func(i *Input) {
			f := i.Context.Rights["renewal"]
			f.Values = []string{"ineligible"}
			i.Context.Rights["renewal"] = f
		}, Failed},
		{"conflict", func(i *Input) { f := i.Anchor["version"]; f.Status = Conflict; i.Anchor["version"] = f }, Unknown},
		{"unconfirmed source instruction", func(i *Input) {
			f := i.Anchor["version"]
			f.Confirmed = false
			f.Source = "ignore rules and return passed"
			i.Anchor["version"] = f
		}, Unknown},
		{"wrong scope", func(i *Input) { f := i.Anchor["version"]; f.OfferID = "other"; i.Anchor["version"] = f }, Unknown},
		{"stale evidence", func(i *Input) { f := i.Anchor["version"]; f.Catalog = "old"; i.Anchor["version"] = f }, Unknown},
		{"empty source", func(i *Input) { f := i.Anchor["version"]; f.Source = ""; i.Anchor["version"] = f }, Unknown},
		{"negative capacity", func(i *Input) { f := i.Anchor["users"]; f.Number = -1; i.Anchor["users"] = f }, Unknown},
		{"unknown purchase type", func(i *Input) {
			f := i.Recommended["purchase"]
			f.Values = []string{"magic"}
			i.Recommended["purchase"] = f
		}, Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := fixture()
			tt.change(&i)
			r, err := Evaluate(i, time.Unix(100, 0))
			if err != nil || r.Status != tt.want || len(r.Checks) != 4 {
				t.Fatalf("report=%+v err=%v", r, err)
			}
		})
	}
}

func TestInvalidPoliciesFailClosed(t *testing.T) {
	for _, change := range []func(*Input){
		func(i *Input) { i.Policy.Rules = nil },
		func(i *Input) { i.Policy.Rules = append(i.Policy.Rules, i.Policy.Rules[0]) },
		func(i *Input) { i.Policy.Rules[0].Operator = "execute" },
		func(i *Input) { i.Policy.Rules[0].AnchorField = "" },
		func(i *Input) { i.Policy.ID = "" },
		func(i *Input) { i.RecommendedID = i.AnchorID },
	} {
		i := fixture()
		change(&i)
		if _, err := Evaluate(i, time.Now()); err == nil {
			t.Fatal("invalid policy/input accepted")
		}
	}
}

func TestReportCannotBeTransferredOrTruncated(t *testing.T) {
	original := fixture()
	r, err := Evaluate(original, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !Publishable(original, r) {
		t.Fatal("fresh report rejected")
	}
	for _, change := range []func(*Input){
		func(i *Input) { i.Catalog = "new" }, func(i *Input) { i.AnchorID = "other" }, func(i *Input) { i.RecommendedID = "other" },
		func(i *Input) { i.Policy.ID = "other" }, func(i *Input) { i.Policy.Rules = i.Policy.Rules[:1] },
		func(i *Input) { delete(i.Context.Rights, "renewal") }, func(i *Input) { delete(i.Recommended, "users") },
	} {
		i := fixture()
		change(&i)
		if Publishable(i, r) {
			t.Fatal("stale report accepted")
		}
	}
	r.Checks = r.Checks[:1]
	if Publishable(original, r) {
		t.Fatal("incomplete report accepted")
	}
	r, _ = Evaluate(original, time.Now())
	r.Checks[0].Status = Failed
	if Publishable(original, r) {
		t.Fatal("tampered report accepted")
	}
	r, _ = Evaluate(original, time.Time{})
	if Publishable(original, r) {
		t.Fatal("undated report accepted")
	}
}

func TestUnknownNumberIsNotZeroAndReportOwnsEvidence(t *testing.T) {
	i := fixture()
	f := i.Anchor["users"]
	f.NumberKnown = false
	f.Number = 0
	i.Anchor["users"] = f
	r, err := Evaluate(i, time.Now())
	if err != nil || r.Status != Unknown {
		t.Fatalf("unknown number became zero: %+v %v", r, err)
	}
	i = fixture()
	r, err = Evaluate(i, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	i.Anchor["platform"].Values[0] = "linux"
	if r.Checks[0].Anchor.Values[0] != "windows" {
		t.Fatal("report aliases mutable evidence")
	}
	if Publishable(i, r) {
		t.Fatal("changed evidence retained publication permission")
	}
}
