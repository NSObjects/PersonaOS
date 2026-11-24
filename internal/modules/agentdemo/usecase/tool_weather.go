package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const weatherCityField = "city"

type weatherTool struct {
	client      *http.Client
	geocodeURL  string
	forecastURL string
}

type weatherLocation struct {
	Name      string  `json:"name"`
	Admin1    string  `json:"admin1,omitempty"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type weatherForecast struct {
	Timezone string          `json:"timezone"`
	Current  json.RawMessage `json:"current"`
	Daily    json.RawMessage `json:"daily"`
}

func (t *weatherTool) Info(context.Context) (*schema.ToolInfo, error) {
	return queryToolInfo(weatherToolName, "查询指定城市当前天气和未来三天天气预报。城市未知时先询问用户。数据来源 Open-Meteo，温度摄氏度，天气编码为 WMO。", weatherCityField, "城市名称，可使用英文或中文"), nil
}

func (t *weatherTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	city, ok := parseNetworkQuery(args, weatherCityField)
	if !ok {
		return httpToolArgumentFeedback, nil
	}
	runCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	location, err := t.locate(runCtx, city)
	if err != nil {
		return networkToolFailure(ctx)
	}
	if location == nil {
		return `{"error":"location_not_found","message":"未找到该城市，请确认城市名称。"}`, nil
	}
	forecast, err := t.forecast(runCtx, *location)
	if err != nil || forecast.Timezone == "" || len(forecast.Current) == 0 || len(forecast.Daily) == 0 {
		return networkToolFailure(ctx)
	}
	data, err := json.Marshal(struct {
		Source   string          `json:"source"`
		Location weatherLocation `json:"location"`
		Forecast weatherForecast `json:"forecast"`
	}{Source: "Open-Meteo (https://open-meteo.com/)", Location: *location, Forecast: forecast})
	return string(data), err
}

func (t *weatherTool) locate(ctx context.Context, city string) (*weatherLocation, error) {
	params := url.Values{"name": {city}, "count": {"1"}, "language": {"zh"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, t.geocodeURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Results []weatherLocation `json:"results"`
	}
	if err := readNetworkJSON(ctx, t.client, request, &result); err != nil {
		return nil, err
	}
	if len(result.Results) == 0 {
		return nil, nil
	}
	return &result.Results[0], nil
}

func (t *weatherTool) forecast(ctx context.Context, location weatherLocation) (weatherForecast, error) {
	params := url.Values{
		"latitude":      {strconv.FormatFloat(location.Latitude, 'f', -1, 64)},
		"longitude":     {strconv.FormatFloat(location.Longitude, 'f', -1, 64)},
		"current":       {"temperature_2m,apparent_temperature,weather_code,wind_speed_10m"},
		"daily":         {"temperature_2m_max,temperature_2m_min,weather_code,precipitation_probability_max"},
		"timezone":      {"auto"},
		"forecast_days": {"3"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, t.forecastURL+"?"+params.Encode(), nil)
	if err != nil {
		return weatherForecast{}, err
	}
	var result weatherForecast
	err = readNetworkJSON(ctx, t.client, request, &result)
	return result, err
}
