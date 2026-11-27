package queryinterpretation

import (
	"strings"
	"testing"
)

func TestDecodeStrictResponse(t *testing.T) {
	good := `{"version":"interpretation-v1","source_query":"нужна помощь сотрудникам с компьютерами","status":"resolved","semantic_query":"программа удаленной поддержки"}`
	x, err := Decode([]byte(good), "нужна помощь сотрудникам с компьютерами")
	if err != nil || x.SemanticQuery != "программа удаленной поддержки" {
		t.Fatalf("%+v %v", x, err)
	}
	for _, raw := range []string{
		good + ` {}`, "```json\n" + good + "\n```", strings.Repeat(" ", 4097),
		strings.Replace(good, `"status":"resolved"`, `"status":"resolved","status":"clarify"`, 1),
		strings.Replace(good, `"status":"resolved"`, `"Status":"resolved"`, 1),
		strings.Replace(good, `"status":"resolved"`, `"status":"resolved","offer_id":"123"`, 1),
		strings.Replace(good, `"semantic_query":"программа удаленной поддержки"`, `"semantic_query":null`, 1),
		strings.Replace(good, `"semantic_query":"программа удаленной поддержки"`, `"semantic_query":[]`, 1),
		strings.Replace(good, `"version":"interpretation-v1",`, "", 1),
		strings.Replace(good, "сотрудникам", "клиентам", 1),
		strings.Replace(good, "программа", string([]byte{0xff}), 1),
		`null`, `[]`, `{"version":"interpretation-v1"`,
	} {
		if _, err := Decode([]byte(raw), "нужна помощь сотрудникам с компьютерами"); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
