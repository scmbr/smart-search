package queryparser

import (
	"strings"
	"testing"
)

func TestPlanBoundAndCacheIdentity(t *testing.T) {
	for _, q := range []string{"нужна помощь сотрудникам с компьютерами", strings.Repeat("я", 256)} {
		p, err := Plan(q)
		if err != nil || p.CostUpperNano > QueryCapNano || p.InputTokenUpper < int64(len(q)) || len(p.Key) != 64 {
			t.Fatalf("%+v %v", p, err)
		}
	}
	a, _ := Plan("текст запроса")
	b, _ := Plan("текст  запроса")
	if a.Key == b.Key {
		t.Fatal("original text must be part of cache key")
	}
	for _, q := range []string{"", strings.Repeat("a", 257), "a\nb"} {
		if _, err := Plan(q); err == nil {
			t.Fatal("invalid query accepted")
		}
	}
	if Cost(100, 10) != 17500 {
		t.Fatal("pricing")
	}
}
