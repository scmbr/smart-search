package localconfig

import (
	"errors"
	"os"

	p "github.com/scmbr/smart-search/internal/domain/queryparser"
)

func CheckParserLive() error {
	if os.Getenv("OPENAI_LIVE_ENABLED") != "true" || os.Getenv("OPENAI_API_KEY") == "" {
		return errors.New("live parser requires OPENAI_LIVE_ENABLED=true and API key")
	}
	if model := os.Getenv("OPENAI_GENERATION_MODEL"); model != "" && model != p.Model {
		return errors.New("parser generation model differs from budgeted model")
	}
	if budget := os.Getenv("OPENAI_BUDGET_USD"); budget != "" && budget != "5" && budget != "5.00" {
		return errors.New("parser budget differs from 5 USD")
	}
	return nil
}
