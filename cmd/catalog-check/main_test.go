package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scmbr/smart-search/internal/catalog"
)

func TestRunValidAndInvalidFixtures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		exit  int
		valid bool
	}{
		{"minimal.xml", 0, true}, {"invalid.xml", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			code := run(context.Background(), []string{"-file", "../../testdata/catalog/" + tc.name}, strings.NewReader(""), &output, &diagnostics)
			if code != tc.exit {
				t.Fatalf("exit %d, want %d: %s", code, tc.exit, diagnostics.String())
			}
			var report catalog.Report
			if err := json.Unmarshal(output.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if !report.Complete || report.CanActivate != tc.valid {
				t.Fatalf("unexpected report: %+v", report)
			}
		})
	}
}

func TestRunIncompleteStdin(t *testing.T) {
	var output, diagnostics bytes.Buffer
	code := run(context.Background(), []string{"-file", "-"}, strings.NewReader(`<yml_catalog>`), &output, &diagnostics)
	if code != 1 || diagnostics.Len() == 0 {
		t.Fatalf("incomplete input not reported: %d %s", code, diagnostics.String())
	}
	var report catalog.Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.CanActivate {
		t.Fatalf("incomplete report marked valid: %+v", report)
	}
}
