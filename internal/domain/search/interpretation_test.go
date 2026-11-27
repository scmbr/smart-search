package search

import (
	"strings"
	"testing"
)

func TestInterpretationValidation(t *testing.T) {
	source := "нужно управлять 25 компьютерами сотрудников удаленно"
	good := Interpretation{Version: InterpretationVersion, SourceQuery: source, Status: "resolved", SemanticQuery: "удаленный доступ к 25 компьютерам"}
	if err := ValidateInterpretation(source, good); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"clarify", "unsupported"} {
		x := good
		x.Status = status
		x.SemanticQuery = ""
		if err := ValidateInterpretation(source, x); err != nil {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func(*Interpretation){
		func(x *Interpretation) { x.Version = "future" },
		func(x *Interpretation) { x.SourceQuery = "другой запрос" },
		func(x *Interpretation) { x.Status = "passed" },
		func(x *Interpretation) { x.SemanticQuery = " " },
		func(x *Interpretation) { x.SemanticQuery = "управление 50 компьютерами" },
		func(x *Interpretation) { x.SemanticQuery = "управление компьютерами" },
		func(x *Interpretation) { x.SemanticQuery = "управление 25\nкомпьютерами" },
		func(x *Interpretation) { x.SemanticQuery = string([]byte{0xff}) },
		func(x *Interpretation) { x.SemanticQuery = strings.Repeat("я", 257) },
		func(x *Interpretation) { x.Status = "clarify" },
	} {
		x := good
		mutate(&x)
		if ValidateInterpretation(source, x) == nil {
			t.Fatalf("accepted %+v", x)
		}
	}
}

func TestPrepareClearsUntrustedSemanticQuery(t *testing.T) {
	r, err := Prepare(Request{Query: "сервер", SemanticQuery: "software", Limit: 4})
	if err != nil || r.SemanticQuery != "" || r.VectorQuery() != "сервер" {
		t.Fatalf("%+v %v", r, err)
	}
}
