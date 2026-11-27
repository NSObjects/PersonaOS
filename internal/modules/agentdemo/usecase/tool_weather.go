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

const (
	weatherCityField = "city"
	weatherFailure   = `{"error":"unavailable","message":"这次没查到天气信息呢。不要猜测或在本轮重复调用失败的工具。"}`
)

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

// weatherPlace exposes the resolved place without internal forecast coordinates.
type weatherPlace struct {
	Name    string `json:"name"`
	Admin1  string `json:"admin1,omitempty"`
	Country string `json:"country"`
}

type weatherForecast struct {
	Timezone string `json:"timezone"`
	Current  struct {
		Time        string   `json:"time"`
		Temperature *float64 `json:"temperature_2m"`
		FeelsLike   *float64 `json:"apparent_temperature,omitempty"`
		Code        *int     `json:"weather_code,omitempty"`
		WindSpeed   *float64 `json:"wind_speed_10m,omitempty"`
	} `json:"current"`
	Daily struct {
		Time          []string   `json:"time"`
		Max           []*float64 `json:"temperature_2m_max,omitempty"`
		Min           []*float64 `json:"temperature_2m_min,omitempty"`
		Code          []*int     `json:"weather_code,omitempty"`
		Precipitation []*float64 `json:"precipitation_probability_max,omitempty"`
	} `json:"daily"`
}

func (t *weatherTool) Info(context.Context) (*schema.ToolInfo, error) {
	return queryToolInfo(weatherToolName, "查询城市当前天气、气温、降雨概率及未来三天（含今天）的预报。询问这些实时天气信息时使用。根据当前对话确定城市，仍不明确时询问用户。温度为摄氏度，天气编码为 WMO，来源 Open-Meteo。", weatherCityField, "城市名称，可使用英文或中文"), nil
}

func (t *weatherTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	city, ok := parseNetworkQuery(args, weatherCityField)
	if !ok {
		return httpToolArgumentFeedback, nil
	}
	beginToolFeedback(ctx, ToolFeedback{WaitingMessage: "我看看。", WaitingDelay: defaultToolWaitingDelay})
	runCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	location, err := t.locate(runCtx, city)
	if err != nil {
		return networkToolFailure(ctx, weatherFailure)
	}
	if location == nil {
		return `{"error":"location_not_found","message":"未找到该城市，请确认城市名称。"}`, nil
	}
	forecast, err := t.forecast(runCtx, *location)
	if err != nil || forecast.Timezone == "" || forecast.Current.Time == "" || forecast.Current.Temperature == nil || len(forecast.Daily.Time) == 0 {
		return networkToolFailure(ctx, weatherFailure)
	}
	data, err := json.Marshal(struct {
		Source   string          `json:"source"`
		Location weatherPlace    `json:"location"`
		Forecast weatherForecast `json:"forecast"`
	}{Source: "Open-Meteo (https://open-meteo.com/)", Location: weatherPlace{Name: location.Name, Admin1: location.Admin1, Country: location.Country}, Forecast: forecast})
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
