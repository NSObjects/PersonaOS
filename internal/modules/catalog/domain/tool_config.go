package domain

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// HTTPToolProvider identifies the HTTP execution adapter.
const HTTPToolProvider = "http"

var toolParameterPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,63}$`)

// HTTPToolParameter describes a supported scalar JSON Schema property.
type HTTPToolParameter struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// HTTPToolSchema is the deliberately small supported JSON Schema subset.
type HTTPToolSchema struct {
	Type       string                       `json:"type"`
	Properties map[string]HTTPToolParameter `json:"properties"`
	Required   []string                     `json:"required,omitempty"`
}

// HTTPToolConfig describes an approved endpoint's argument contract.
type HTTPToolConfig struct {
	Method      string         `json:"method"`
	InputSchema HTTPToolSchema `json:"input_schema"`
}

// ParseHTTPToolConfig validates metadata without making network requests.
func ParseHTTPToolConfig(raw string) (HTTPToolConfig, error) {
	var cfg HTTPToolConfig
	if len(raw) == 0 || len(raw) > 8192 {
		return cfg, ErrInvalidResource
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("%w: invalid HTTP tool JSON", ErrInvalidResource)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return cfg, ErrInvalidResource
	}
	if (cfg.Method != "GET" && cfg.Method != "POST") || cfg.InputSchema.Type != "object" ||
		cfg.InputSchema.Properties == nil || len(cfg.InputSchema.Properties) > 32 {
		return cfg, ErrInvalidResource
	}
	for key, param := range cfg.InputSchema.Properties {
		if !toolParameterPattern.MatchString(key) || len(param.Description) > 512 {
			return cfg, ErrInvalidResource
		}
		switch param.Type {
		case "string", "number", "integer", "boolean":
		default:
			return cfg, ErrInvalidResource
		}
	}
	seen := make(map[string]bool)
	for _, name := range cfg.InputSchema.Required {
		if _, ok := cfg.InputSchema.Properties[name]; !ok || seen[name] {
			return cfg, ErrInvalidResource
		}
		seen[name] = true
	}
	return cfg, nil
}

// WithToolConfig validates and attaches immutable tool configuration text.
func (r Resource) WithToolConfig(raw string) (Resource, error) {
	raw = strings.TrimSpace(raw)
	if r.Kind == ResourceTool && r.Provider == HTTPToolProvider {
		if _, err := ParseHTTPToolConfig(raw); err != nil {
			return Resource{}, err
		}
		if strings.TrimSpace(r.Description) == "" || !resourceCodePattern.MatchString(r.Reference) {
			return Resource{}, ErrInvalidResource
		}
	} else if raw != "" {
		return Resource{}, ErrInvalidResource
	}
	r.ToolConfig = raw
	return r, nil
}
