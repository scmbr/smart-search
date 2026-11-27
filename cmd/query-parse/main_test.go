package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestPlanWorksWithoutSecretsDatabaseOrAPI(t *testing.T) {
	t.Setenv("OPENAI_LIVE_ENABLED", "false")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("POSTGRES_DSN", "")
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-query", "нужно помогать сотрудникам с компьютерами"}, &out); err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Mode != "plan" || r.Execution.Requests != 0 || r.Plan.CostUpperNano < 1 {
		t.Fatalf("%+v %v", r, err)
	}
	for _, args := range [][]string{{"-query", "текст", "-mode", "unknown"}, {"-query", "текст", "-max-cost-nano", "1"}, {"-query", "текст", "-max-cost-nano", "1000001"}} {
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatal("invalid flags accepted")
		}
	}
}
