// Package queryparser owns cache identity and conservative API cost bounds.
package queryparser

import (
	"crypto/sha256"
	"encoding/hex"

	d "github.com/scmbr/smart-search/internal/domain/search"
)

const Model = "gpt-6-luna"
const PromptVersion = "parser-prompt-v1"
const PolicyVersion = "parser-runtime-v1"
const MaxOutputTokens int64 = 512
const QueryCapNano int64 = 1000000
const ProjectCapNano int64 = 5000000000

// Includes system prompt, schema and message framing; the SDK test verifies it.
const FixedInputTokenUpper int64 = 4096

type Estimate struct {
	Key, Model, PromptVersion, PolicyVersion        string
	InputTokenUpper, MaxOutputTokens, CostUpperNano int64
}

func Plan(query string) (Estimate, error) {
	if err := d.ValidateInterpretation(query, d.Interpretation{Version: d.InterpretationVersion, SourceQuery: query, Status: "clarify"}); err != nil {
		return Estimate{}, err
	}
	sum := sha256.Sum256([]byte(PolicyVersion + "\x00" + Model + "\x00" + PromptVersion + "\x00" + d.InterpretationVersion + "\x00" + query))
	upper := FixedInputTokenUpper + int64(len(query))
	return Estimate{Key: hex.EncodeToString(sum[:]), Model: Model, PromptVersion: PromptVersion, PolicyVersion: PolicyVersion, InputTokenUpper: upper, MaxOutputTokens: MaxOutputTokens, CostUpperNano: Cost(upper, MaxOutputTokens)}, nil
}

// Cost uses the cache-write input rate for every token, without cache discounts.
func Cost(input, output int64) int64 { return input*125 + output*500 }
