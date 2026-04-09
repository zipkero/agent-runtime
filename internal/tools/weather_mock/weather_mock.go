package weather_mock

import (
	"context"
	"fmt"
	"strings"

	"agentflow/internal/tools"
	"agentflow/internal/types"
)

// weatherData 는 도시별 고정 날씨 데이터다.
type weatherData struct {
	Condition   string
	TempCelsius int
	Humidity    int
}

var mockDB = map[string]weatherData{
	"seoul":    {Condition: "맑음", TempCelsius: 18, Humidity: 45},
	"busan":    {Condition: "흐림", TempCelsius: 20, Humidity: 60},
	"jeju":     {Condition: "비", TempCelsius: 16, Humidity: 80},
	"incheon":  {Condition: "맑음", TempCelsius: 17, Humidity: 50},
	"daejeon":  {Condition: "안개", TempCelsius: 15, Humidity: 75},
	"tokyo":    {Condition: "맑음", TempCelsius: 22, Humidity: 55},
	"newyork":  {Condition: "흐림", TempCelsius: 12, Humidity: 65},
	"london":   {Condition: "비", TempCelsius: 10, Humidity: 85},
	"paris":    {Condition: "맑음", TempCelsius: 19, Humidity: 48},
	"shanghai": {Condition: "흐림", TempCelsius: 25, Humidity: 70},
}

// Kind 는 WorkingMemory 분류 키다.
const Kind = "weather"

// WeatherMock 은 도시 이름을 받아 고정된 날씨 데이터를 반환하는 mock Tool 구현체다.
type WeatherMock struct{}

func New() *WeatherMock {
	return &WeatherMock{}
}

func (w *WeatherMock) Name() string {
	return "weather_mock"
}

func (w *WeatherMock) Description() string {
	return "도시 이름을 받아 현재 날씨 정보를 반환한다. 테스트용 mock 데이터를 사용한다."
}

func (w *WeatherMock) InputSchema() tools.Schema {
	return tools.Schema{
		Fields: []tools.FieldSchema{
			{
				Name:        "city",
				Type:        tools.FieldTypeString,
				Description: "날씨를 조회할 도시 이름 (예: 'Seoul', 'Tokyo')",
				Required:    true,
			},
		},
	}
}

func (w *WeatherMock) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	raw, ok := input["city"]
	if !ok {
		return types.ToolResult{ToolName: w.Name(), Kind: Kind, IsError: true, ErrMsg: "city 필드가 없습니다"}, nil
	}
	city, ok := raw.(string)
	if !ok {
		return types.ToolResult{ToolName: w.Name(), Kind: Kind, IsError: true, ErrMsg: "city 는 string 이어야 합니다"}, nil
	}

	key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(city), " ", ""))
	data, found := mockDB[key]
	if !found {
		return types.ToolResult{
			ToolName: w.Name(),
			Kind:     Kind,
			IsError:  true,
			ErrMsg:   fmt.Sprintf("'%s' 에 대한 날씨 데이터가 없습니다", city),
		}, nil
	}

	output := fmt.Sprintf("도시: %s | 날씨: %s | 기온: %d°C | 습도: %d%%",
		city, data.Condition, data.TempCelsius, data.Humidity)

	return types.ToolResult{
		ToolName: w.Name(),
		Kind:     Kind,
		Output:   output,
	}, nil
}
