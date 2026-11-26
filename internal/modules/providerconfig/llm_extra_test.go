package providerconfig

import "testing"

const testThinkingOff = `{"enable_thinking":false}`

func TestLLMExtraFields(t *testing.T) {
	service, ok := Lookup("llm")
	if !ok {
		t.Fatal("missing LLM service")
	}
	values, err := service.Merge(nil, Values{FieldLLMExtraFields: testThinkingOff})
	if err != nil {
		t.Fatal(err)
	}
	view := service.Project(values, "database")
	fields, err := ParseLLMExtraFields(view.Values[FieldLLMExtraFields])
	if err != nil || fields["enable_thinking"] != false {
		t.Fatalf("thinking option=%v err=%v", fields, err)
	}
	cleared, err := service.Merge(values, Values{FieldLLMExtraFields: ""})
	if err != nil || cleared[FieldLLMExtraFields] != "" {
		t.Fatal("explicit clear failed")
	}
	for _, raw := range []string{"", "{}"} {
		if _, err := ParseLLMExtraFields(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		"null", "[]", "true", "{", `{"model":"other"}`, `{"messages":[]}`,
		`{"stream":false}`, `{"tools":[]}`, `{"api_key":"not-allowed"}`,
	} {
		if _, err := service.Merge(values, Values{FieldLLMExtraFields: raw}); err == nil {
			t.Fatalf("accepted invalid extra fields %s", raw)
		}
	}
}
