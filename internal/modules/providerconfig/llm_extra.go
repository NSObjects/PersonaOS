package providerconfig

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	// FieldLLMExtraFields is the persisted vendor request options field.
	FieldLLMExtraFields = "extra_fields"
	// EnvLLMExtraFields is the legacy environment fallback for request options.
	EnvLLMExtraFields = "PERSONAOS_LLM_EXTRA_FIELDS"
)

// ParseLLMExtraFields accepts an optional JSON object of vendor request options.
// Agent-owned fields must not be overridden by the SDK's ExtraFields mechanism.
func ParseLLMExtraFields(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var fields map[string]any
	if len(raw) > 4096 || json.Unmarshal([]byte(raw), &fields) != nil || fields == nil {
		return nil, errors.New("LLM extra fields must be a JSON object of at most 4096 bytes")
	}
	for key := range fields {
		switch key {
		case "", "model", "messages", "tools", "tool_choice", "functions", "function_call",
			"stream", "stream_options", "parallel_tool_calls", "n", "response_format",
			"modalities", "audio", "api_key", "authorization", "base_url":
			return nil, errors.New("LLM extra fields cannot override runtime-managed fields")
		}
	}
	return fields, nil
}
