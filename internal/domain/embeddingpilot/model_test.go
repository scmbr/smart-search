package embeddingpilot

import (
	"math"
	"strings"
	"testing"
)

func TestBoundsKeysAndCosine(t *testing.T) {
	if n, err := TokenBound("ёж"); err != nil || n != 20 {
		t.Fatalf("bound=%d %v", n, err)
	}
	for _, s := range []string{"", strings.Repeat("x", 2049)} {
		if _, err := TokenBound(s); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if CacheKey("a") == CacheKey("b") {
		t.Fatal("cache collision")
	}
	if s, err := Cosine([]float64{1, 0}, []float64{0, 1}); err != nil || s != 0 {
		t.Fatal(s, err)
	}
	for _, v := range [][]float64{{0, 0}, {math.NaN(), 1}, {math.Inf(1), 0}, {1e308, 1e308}, {1}} {
		if _, err := Cosine(v, []float64{1, 0}); err == nil {
			t.Fatal("invalid vector accepted")
		}
	}
	if Cost(1000000) != 20000000 {
		t.Fatal("price is not $0.02 / 1M tokens")
	}
}
