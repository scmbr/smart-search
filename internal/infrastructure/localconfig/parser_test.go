package localconfig

import (
	"testing"

	p "github.com/scmbr/smart-search/internal/domain/queryparser"
)

func TestParserLiveRequiresExplicitConfiguration(t *testing.T) {
	t.Setenv("OPENAI_LIVE_ENABLED", "false")
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("OPENAI_GENERATION_MODEL", p.Model)
	t.Setenv("OPENAI_BUDGET_USD", "5")
	if CheckParserLive() == nil {
		t.Fatal("live disabled")
	}
	t.Setenv("OPENAI_LIVE_ENABLED", "true")
	if err := CheckParserLive(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_GENERATION_MODEL", "other")
	if CheckParserLive() == nil {
		t.Fatal("unpriced model")
	}
	t.Setenv("OPENAI_GENERATION_MODEL", p.Model)
	t.Setenv("OPENAI_BUDGET_USD", "10")
	if CheckParserLive() == nil {
		t.Fatal("budget override")
	}
}
