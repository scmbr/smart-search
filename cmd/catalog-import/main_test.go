package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCLIRejectsInvalidConfigurationWithoutLeakingDSN(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("CATALOG_PATH", "")
	t.Setenv("POSTGRES_DSN", "")
	for _, args := range [][]string{{}, {"-batch-size", "0"}, {"-batch-size", "5001"}, {"-migrate-only", "unexpected"}, {"-migrate-only"}} {
		var out, diagnostics bytes.Buffer
		if code := run(context.Background(), args, &out, &diagnostics); code != 1 || diagnostics.Len() == 0 {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, diagnostics.String())
		}
	}
	t.Setenv("POSTGRES_DSN", "postgres://name:private-test-secret@host:badport/db")
	var out, diagnostics bytes.Buffer
	if code := run(context.Background(), []string{"-migrate-only"}, &out, &diagnostics); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(diagnostics.String(), "private-test-secret") {
		t.Fatal("DSN secret in diagnostics")
	}
}
