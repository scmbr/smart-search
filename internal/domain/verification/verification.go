// Package verification evaluates structured, scoped evidence without network or model calls.
package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
)

type Status string

const (
	Passed   Status = "passed"
	Failed   Status = "failed"
	Unknown  Status = "unknown"
	Known    Status = "known"
	Conflict Status = "conflict"
)

type Operator string

const (
	Equals          Operator = "equals"
	Contains        Operator = "contains"
	AtLeast         Operator = "at_least"
	LicenseEligible Operator = "license_eligible"
)

// Fact must be constructed by a trusted adapter; Confirmed is not a client assertion.
// Values contain normalized identifiers, never free-text compatibility instructions.
type Fact struct {
	Status           Status
	Values           []string
	Number           int64
	NumberKnown      bool
	Unit             string
	Source           string
	Confirmed        bool
	OfferID, Catalog string
}
type Rule struct {
	ID                            string
	Operator                      Operator
	AnchorField, RecommendedField string
}
type Policy struct {
	ID    string
	Rules []Rule
}
type Context struct {
	ID     string
	Rights map[string]Fact
}
type Input struct {
	AnchorID, RecommendedID, Catalog string
	Policy                           Policy
	Anchor, Recommended              map[string]Fact
	Context                          Context
}
type Check struct {
	RuleID                     string
	Status                     Status
	Reason                     string
	Anchor, Recommended, Right Fact
}
type Report struct {
	PolicyID, AnchorID, RecommendedID, Catalog, InputSHA256 string
	CheckedAt                                               time.Time
	Status                                                  Status
	Checks                                                  []Check
}

var ErrInvalid = errors.New("invalid verification input or policy")

func valid(i Input) bool {
	if strings.TrimSpace(i.AnchorID) == "" || strings.TrimSpace(i.RecommendedID) == "" || i.AnchorID == i.RecommendedID || strings.TrimSpace(i.Catalog) == "" || strings.TrimSpace(i.Policy.ID) == "" || len(i.Policy.Rules) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range i.Policy.Rules {
		if strings.TrimSpace(r.ID) == "" || seen[r.ID] || strings.TrimSpace(r.RecommendedField) == "" {
			return false
		}
		seen[r.ID] = true
		switch r.Operator {
		case Equals, Contains, AtLeast:
			if strings.TrimSpace(r.AnchorField) == "" {
				return false
			}
		case LicenseEligible:
			if r.AnchorField != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func usable(f Fact, offer, catalog string) bool {
	return f.Status == Known && f.Confirmed && strings.TrimSpace(f.Source) != "" && f.OfferID == offer && f.Catalog == catalog
}
func scalar(f Fact) bool { return len(f.Values) == 1 && strings.TrimSpace(f.Values[0]) != "" }

func check(i Input, r Rule) Check {
	a, b := i.Anchor[r.AnchorField], i.Recommended[r.RecommendedField]
	c := Check{RuleID: r.ID, Status: Unknown, Reason: "missing_or_unconfirmed_evidence", Anchor: a, Recommended: b}
	if b.Status == Conflict || (r.Operator != LicenseEligible && a.Status == Conflict) {
		c.Reason = "conflicting_evidence"
		return c
	}
	if !usable(b, i.RecommendedID, i.Catalog) {
		return c
	}
	if r.Operator == LicenseEligible {
		if !scalar(b) {
			return c
		}
		kind := b.Values[0]
		switch kind {
		case "new", "renewal", "upgrade":
		default:
			return c
		}
		right := i.Context.Rights[kind]
		c.Right = right
		if right.Status == Conflict {
			c.Reason = "conflicting_evidence"
			return c
		}
		if !usable(right, i.AnchorID, i.Catalog) || !scalar(right) {
			return c
		}
		switch right.Values[0] {
		case "eligible":
			c.Status = Passed
		case "ineligible":
			c.Status = Failed
		default:
			return c
		}
	} else {
		if !usable(a, i.AnchorID, i.Catalog) {
			return c
		}
		matches := false
		switch r.Operator {
		case Equals:
			if !scalar(a) || !scalar(b) {
				return c
			}
			matches = a.Values[0] == b.Values[0]
		case Contains:
			if !scalar(a) || len(b.Values) == 0 {
				return c
			}
			for _, v := range b.Values {
				if strings.TrimSpace(v) == "" {
					return c
				}
				if v == a.Values[0] {
					matches = true
				}
			}
		case AtLeast:
			if !a.NumberKnown || !b.NumberKnown || len(a.Values) != 0 || len(b.Values) != 0 || a.Number < 0 || b.Number < 0 || strings.TrimSpace(a.Unit) == "" || a.Unit != b.Unit {
				return c
			}
			matches = b.Number >= a.Number
		}
		c.Status = Failed
		if matches {
			c.Status = Passed
		}
	}
	c.Reason = "matched"
	if c.Status == Failed {
		c.Reason = "mismatch"
	}
	return c
}

// Evaluate runs every mandatory rule; a failed rule dominates unknown in the summary.
func Evaluate(i Input, at time.Time) (Report, error) {
	if !valid(i) {
		return Report{}, ErrInvalid
	}
	data, err := json.Marshal(i)
	if err != nil {
		return Report{}, err
	}
	sum := sha256.Sum256(data)
	r := Report{PolicyID: i.Policy.ID, AnchorID: i.AnchorID, RecommendedID: i.RecommendedID, Catalog: i.Catalog, InputSHA256: hex.EncodeToString(sum[:]), CheckedAt: at.UTC(), Status: Passed}
	for _, rule := range i.Policy.Rules {
		c := check(i, rule)
		r.Checks = append(r.Checks, c)
		if c.Status == Failed {
			r.Status = Failed
		} else if c.Status == Unknown && r.Status != Failed {
			r.Status = Unknown
		}
	}
	// Own all evidence slices: later caller mutation must not silently change the report.
	encoded, err := json.Marshal(r)
	if err != nil {
		return Report{}, err
	}
	var owned Report
	if err = json.Unmarshal(encoded, &owned); err != nil {
		return Report{}, err
	}
	return owned, nil
}

// Publishable recomputes the full report against trusted current inputs.
// It is not an authenticity signature and must never validate client-supplied inputs.
func Publishable(i Input, r Report) bool {
	if r.Status != Passed || r.CheckedAt.IsZero() {
		return false
	}
	expected, err := Evaluate(i, r.CheckedAt)
	return err == nil && expected.Status == Passed && reflect.DeepEqual(expected, r)
}
