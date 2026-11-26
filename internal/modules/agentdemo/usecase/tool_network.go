package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const (
	weatherToolName   = "get_weather"
	webSearchToolName = "web_search"
	maxQueryBytes     = 500
	maxNetworkBody    = 64 << 10
	networkFailure    = `{"error":"unavailable","message":"暂时无法获取工具结果，请如实告知用户，不要猜测或在本轮重复调用失败的工具。"}`
)

func queryToolInfo(name, desc, field, fieldDesc string) *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: name, Desc: desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			field: {Type: schema.String, Desc: fieldDesc, Required: true},
		}),
	}
}

func parseNetworkQuery(args, field string) (string, bool) {
	var fields map[string]string
	if len(args) > 2048 || json.Unmarshal([]byte(args), &fields) != nil || len(fields) != 1 {
		return "", false
	}
	value := strings.TrimSpace(fields[field])
	return value, value != "" && len(value) <= maxQueryBytes
}

func readNetworkJSON(ctx context.Context, client *http.Client, request *http.Request, output any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("network tool request failed")
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			client.CloseIdleConnections()
		}
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return errors.New("network tool provider rejected request")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxNetworkBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxNetworkBody {
		return errors.New("network tool response too large")
	}
	return json.Unmarshal(body, output)
}

func networkToolFailure(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return networkFailure, nil
}
