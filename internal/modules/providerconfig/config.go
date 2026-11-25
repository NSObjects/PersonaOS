// Package providerconfig owns external service credentials, separate from
// ordinary system settings and device capability resources.
package providerconfig

import (
	"errors"
	"net/url"
	"strings"
)

const (
	// FieldAPIKey names the credential used by single-key providers.
	FieldAPIKey = "api_key"
	// ServiceTencent identifies Tencent Cloud.
	ServiceTencent = "tencent"
)

// Field declares an allowed configuration field and its environment fallback.
type Field struct {
	Name   string `json:"name"`
	Env    string `json:"-"`
	Secret bool   `json:"secret"`
}

// Service declares one independently replaceable configuration group.
type Service struct {
	ID     string  `json:"id"`
	Fields []Field `json:"fields"`
}

// Services returns the supported groups in management display order.
func Services() []Service {
	return []Service{
		{ID: "llm", Fields: []Field{
			{Name: FieldAPIKey, Env: "PERSONAOS_LLM_API_KEY", Secret: true},
			{Name: "model", Env: "PERSONAOS_LLM_MODEL"},
			{Name: "base_url", Env: "PERSONAOS_LLM_BASE_URL"},
		}},
		{ID: "volcengine", Fields: []Field{
			{Name: "speech_api_key", Env: "VOLCENGINE_API_KEY", Secret: true},
			{Name: "ark_api_key", Env: "PERSONAOS_EMBEDDING_API_KEY", Secret: true},
			{Name: "asr_resource_id", Env: "VOLCENGINE_ASR_RESOURCE_ID"},
			{Name: "tts_model_id", Env: "VOLCENGINE_TTS_MODEL_ID"},
		}},
		{ID: "aliyun", Fields: []Field{
			{Name: FieldAPIKey, Env: "DASHSCOPE_API_KEY", Secret: true},
		}},
		{ID: ServiceTencent, Fields: []Field{
			{Name: "app_id", Env: "TENCENT_APP_ID"},
			{Name: "secret_id", Env: "TENCENT_SECRET_ID", Secret: true},
			{Name: "secret_key", Env: "TENCENT_SECRET_KEY", Secret: true},
		}},
		{ID: "deepseek", Fields: []Field{
			{Name: FieldAPIKey, Env: "DEEPSEEK_API_KEY", Secret: true},
		}},
		{ID: "asr", Fields: []Field{
			{Name: "provider", Env: "PERSONAOS_ASR_PROVIDER"},
		}},
	}
}

// Lookup returns a copy of a supported service definition.
func Lookup(id string) (Service, bool) {
	for _, service := range Services() {
		if service.ID == id {
			return service, true
		}
	}
	return Service{}, false
}

// Values contains internal credentials. Never serialize this type to HTTP.
type Values map[string]string

// View is the only projection intended for management responses.
type View struct {
	ID      string            `json:"id"`
	Source  string            `json:"source"`
	Values  map[string]string `json:"values"`
	Secrets map[string]bool   `json:"secrets"`
}

// Project exposes configuration values and only presence flags for secrets.
func (s Service) Project(values Values, source string) View {
	view := View{
		ID:      s.ID,
		Source:  source,
		Values:  make(map[string]string),
		Secrets: make(map[string]bool),
	}
	for _, field := range s.Fields {
		if field.Secret {
			view.Secrets[field.Name] = values[field.Name] != ""
		} else {
			view.Values[field.Name] = values[field.Name]
		}
	}
	return view
}

// Environment loads only the group's explicit allowlist.
func (s Service) Environment(getenv func(string) string) Values {
	values := make(Values, len(s.Fields))
	for _, field := range s.Fields {
		values[field.Name] = getenv(field.Env)
	}
	return values
}

// Merge applies explicit changes. Omitted fields retain their previous value;
// an explicit empty string clears a value. Unknown fields are rejected.
func (s Service) Merge(current, changes Values) (Values, error) {
	allowed := make(map[string]bool, len(s.Fields))
	next := make(Values, len(s.Fields))
	for _, field := range s.Fields {
		allowed[field.Name] = true
		next[field.Name] = current[field.Name]
	}
	for key, value := range changes {
		if !allowed[key] {
			return nil, errors.New("unsupported provider configuration field")
		}
		next[key] = strings.TrimSpace(value)
	}
	if err := s.Validate(next); err != nil {
		return nil, err
	}
	return next, nil
}

// Validate checks structural completeness without contacting paid APIs.
func (s Service) Validate(values Values) error {
	for _, field := range s.Fields {
		value := values[field.Name]
		if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("invalid provider configuration value")
		}
	}
	switch s.ID {
	case "llm":
		if (values[FieldAPIKey] == "") != (values["model"] == "") {
			return errors.New("LLM requires both API key and model")
		}
		return validateBaseURL(values["base_url"])
	case ServiceTencent:
		count := 0
		for _, key := range []string{"app_id", "secret_id", "secret_key"} {
			if values[key] != "" {
				count++
			}
		}
		if count != 0 && count != 3 {
			return errors.New("tencent requires app ID, secret ID and secret key")
		}
	case "asr":
		switch values["provider"] {
		case "", "aliyun", "volcengine", ServiceTencent:
		default:
			return errors.New("unsupported ASR provider")
		}
	}
	return nil
}

func validateBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("invalid LLM base URL")
	}
	return nil
}
