package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

type httpTool struct {
	info     *schema.ToolInfo
	endpoint url.URL
	config   domain.HTTPToolConfig
	client   *http.Client
}

func newHTTPTool(resource domain.Resource, endpoint string, client *http.Client) (tool.BaseTool, error) {
	cfg, err := domain.ParseHTTPToolConfig(resource.ToolConfig)
	if err != nil {
		return nil, err
	}
	address, err := url.Parse(endpoint)
	if err != nil || address.Host == "" || (address.Scheme != "http" && address.Scheme != "https") ||
		address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		return nil, fmt.Errorf("HTTP tool endpoint is not approved or invalid")
	}
	params := make(map[string]*schema.ParameterInfo, len(cfg.InputSchema.Properties))
	for name, property := range cfg.InputSchema.Properties {
		params[name] = &schema.ParameterInfo{Type: schema.DataType(property.Type), Desc: property.Description}
	}
	for _, name := range cfg.InputSchema.Required {
		params[name].Required = true
	}
	return &httpTool{
		info:     &schema.ToolInfo{Name: resource.Code, Desc: resource.Description, ParamsOneOf: schema.NewParamsOneOfByParams(params)},
		endpoint: *address,
		config:   cfg,
		client:   client,
	}, nil
}

func (t *httpTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *httpTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	values, err := t.arguments(args)
	if err != nil {
		return "", err
	}
	address := t.endpoint
	var body io.Reader
	if t.config.Method == http.MethodGet {
		query := address.Query()
		for key, value := range values {
			query.Set(key, fmt.Sprint(value))
		}
		address.RawQuery = query.Encode()
	} else {
		body = strings.NewReader(args)
	}
	request, err := http.NewRequestWithContext(ctx, t.config.Method, address.String(), body)
	if err != nil {
		return "", fmt.Errorf("build HTTP tool request")
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := t.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("HTTP tool request failed")
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			// The response body is not reusable after a close failure.
			t.client.CloseIdleConnections()
		}
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP tool returned status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil {
		return "", fmt.Errorf("read HTTP tool response: %w", err)
	}
	if len(data) > 16384 || !utf8.Valid(data) {
		return "", fmt.Errorf("HTTP tool response exceeds text limits")
	}
	return string(data), nil
}

func (t *httpTool) arguments(args string) (map[string]any, error) {
	if len(args) > 8192 || !json.Valid([]byte(args)) {
		return nil, fmt.Errorf("invalid HTTP tool arguments")
	}
	decoder := json.NewDecoder(strings.NewReader(args))
	decoder.UseNumber()
	var values map[string]any
	if err := decoder.Decode(&values); err != nil || values == nil {
		return nil, fmt.Errorf("HTTP tool arguments must be an object")
	}
	for key, value := range values {
		param, ok := t.config.InputSchema.Properties[key]
		if !ok || !matchesToolParameter(param.Type, value) {
			return nil, fmt.Errorf("invalid HTTP tool parameter")
		}
	}
	for _, key := range t.config.InputSchema.Required {
		if _, ok := values[key]; !ok {
			return nil, fmt.Errorf("missing required HTTP tool parameter")
		}
	}
	return values, nil
}

func matchesToolParameter(kind string, value any) bool {
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number", "integer":
		n, ok := value.(json.Number)
		if !ok {
			return false
		}
		if kind == "integer" {
			_, err := strconv.ParseInt(n.String(), 10, 64)
			return err == nil
		}
		_, err := strconv.ParseFloat(n.String(), 64)
		return err == nil
	default:
		return false
	}
}
