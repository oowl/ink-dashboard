package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"ink-dashboard/server/internal/config"
)

type openMeteoResponse struct {
	Current struct {
		Temperature float64 `json:"temperature_2m"`
		WeatherCode int     `json:"weather_code"`
		WindSpeed   float64 `json:"wind_speed_10m"`
	} `json:"current"`
	Daily struct {
		Max []float64 `json:"temperature_2m_max"`
		Min []float64 `json:"temperature_2m_min"`
	} `json:"daily"`
}

func (p *RealProvider) FetchWeather(ctx context.Context) (Weather, error) {
	if p.cfg.WeatherProvider == "caiyun" {
		return p.FetchCaiyunWeather(ctx)
	}
	return p.FetchOpenMeteoWeather(ctx)
}

func (p *RealProvider) FetchOpenMeteoWeather(ctx context.Context) (Weather, error) {
	endpoint := "https://api.open-meteo.com/v1/forecast"
	values := url.Values{}
	values.Set("latitude", fmt.Sprintf("%.6f", p.cfg.WeatherLatitude))
	values.Set("longitude", fmt.Sprintf("%.6f", p.cfg.WeatherLongitude))
	values.Set("current", "temperature_2m,weather_code,wind_speed_10m")
	values.Set("daily", "temperature_2m_max,temperature_2m_min")
	values.Set("forecast_days", "1")
	values.Set("timezone", p.cfg.Timezone)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return Weather{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return Weather{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Weather{}, fmt.Errorf("open-meteo HTTP %d", resp.StatusCode)
	}

	var decoded openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Weather{}, err
	}

	highLow := ""
	if len(decoded.Daily.Max) > 0 && len(decoded.Daily.Min) > 0 {
		highLow = formatHighLow(decoded.Daily.Max[0], decoded.Daily.Min[0], p.cfg.Language)
	}

	location := p.cfg.WeatherLocation
	if location == "" || location == "Weather" {
		location = fmt.Sprintf("%.2f, %.2f", p.cfg.WeatherLatitude, p.cfg.WeatherLongitude)
	}

	return Weather{
		Location:    location,
		Condition:   weatherCodeName(decoded.Current.WeatherCode, p.cfg.Language),
		Temperature: formatTemperature(decoded.Current.Temperature, p.cfg.Language),
		HighLow:     highLow,
		Wind:        formatWind(decoded.Current.WindSpeed, p.cfg.Language),
	}, nil
}

type caiyunWeatherResponse struct {
	Status string `json:"status"`
	Result struct {
		Realtime struct {
			Status      string  `json:"status"`
			Temperature float64 `json:"temperature"`
			Skycon      string  `json:"skycon"`
			Humidity    float64 `json:"humidity"`
			Wind        struct {
				Speed float64 `json:"speed"`
			} `json:"wind"`
			AirQuality struct {
				AQI struct {
					CHN float64 `json:"chn"`
				} `json:"aqi"`
				Description struct {
					CHN string `json:"chn"`
				} `json:"description"`
			} `json:"air_quality"`
		} `json:"realtime"`
		Daily struct {
			Temperature []struct {
				Max float64 `json:"max"`
				Min float64 `json:"min"`
			} `json:"temperature"`
		} `json:"daily"`
		ForecastKeypoint string `json:"forecast_keypoint"`
	} `json:"result"`
}

func (p *RealProvider) FetchCaiyunWeather(ctx context.Context) (Weather, error) {
	if p.cfg.CaiyunToken == "" {
		return Weather{}, fmt.Errorf("missing Caiyun token")
	}

	endpoint := fmt.Sprintf(
		"https://api.caiyunapp.com/v2.6/%s/%.6f,%.6f/weather",
		url.PathEscape(p.cfg.CaiyunToken),
		p.cfg.WeatherLongitude,
		p.cfg.WeatherLatitude,
	)
	values := url.Values{}
	values.Set("dailysteps", "1")
	values.Set("hourlysteps", "1")
	values.Set("alert", "false")
	values.Set("lang", p.cfg.CaiyunLang)
	values.Set("unit", p.cfg.CaiyunUnit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return Weather{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return Weather{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Weather{}, fmt.Errorf("caiyun HTTP %d", resp.StatusCode)
	}

	var decoded caiyunWeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Weather{}, err
	}
	if decoded.Status != "" && decoded.Status != "ok" {
		return Weather{}, fmt.Errorf("caiyun status %s", decoded.Status)
	}

	location := p.cfg.WeatherLocation
	if location == "" || location == "Weather" {
		location = fmt.Sprintf("%.2f, %.2f", p.cfg.WeatherLatitude, p.cfg.WeatherLongitude)
	}

	highLow := ""
	if len(decoded.Result.Daily.Temperature) > 0 {
		highLow = formatHighLow(decoded.Result.Daily.Temperature[0].Max, decoded.Result.Daily.Temperature[0].Min, p.cfg.Language)
	}
	if highLow == "" && decoded.Result.Realtime.AirQuality.AQI.CHN > 0 {
		highLow = fmt.Sprintf("AQI %.0f", decoded.Result.Realtime.AirQuality.AQI.CHN)
	}

	wind := formatWind(decoded.Result.Realtime.Wind.Speed, p.cfg.Language)
	if decoded.Result.Realtime.AirQuality.Description.CHN != "" {
		wind = wind + " · " + decoded.Result.Realtime.AirQuality.Description.CHN
	}

	return Weather{
		Location:    location,
		Condition:   caiyunSkyconName(decoded.Result.Realtime.Skycon, p.cfg.Language),
		Temperature: formatTemperature(decoded.Result.Realtime.Temperature, p.cfg.Language),
		HighLow:     highLow,
		Wind:        wind,
	}, nil
}

func formatTemperature(value float64, language string) string {
	if config.IsChinese(language) {
		return fmt.Sprintf("%.0f°C", value)
	}
	return fmt.Sprintf("%.0f C", value)
}

func formatHighLow(high, low float64, language string) string {
	if config.IsChinese(language) {
		return fmt.Sprintf("高 %.0f / 低 %.0f", high, low)
	}
	return fmt.Sprintf("H %.0f / L %.0f", high, low)
}

func formatWind(speed float64, language string) string {
	if config.IsChinese(language) {
		return fmt.Sprintf("风 %.0f km/h", speed)
	}
	return fmt.Sprintf("Wind %.0f km/h", speed)
}

func caiyunSkyconName(skycon string, language string) string {
	if !config.IsChinese(language) {
		switch skycon {
		case "CLEAR_DAY", "CLEAR_NIGHT":
			return "Clear"
		case "PARTLY_CLOUDY_DAY", "PARTLY_CLOUDY_NIGHT":
			return "Partly cloudy"
		case "CLOUDY":
			return "Cloudy"
		case "LIGHT_HAZE":
			return "Light haze"
		case "MODERATE_HAZE":
			return "Moderate haze"
		case "HEAVY_HAZE":
			return "Heavy haze"
		case "LIGHT_RAIN":
			return "Light rain"
		case "MODERATE_RAIN":
			return "Rain"
		case "HEAVY_RAIN":
			return "Heavy rain"
		case "STORM_RAIN":
			return "Storm rain"
		case "FOG":
			return "Fog"
		case "LIGHT_SNOW":
			return "Light snow"
		case "MODERATE_SNOW":
			return "Snow"
		case "HEAVY_SNOW":
			return "Heavy snow"
		case "STORM_SNOW":
			return "Storm snow"
		case "DUST":
			return "Dust"
		case "SAND":
			return "Sand"
		case "WIND":
			return "Wind"
		default:
			if skycon == "" {
				return "Unknown"
			}
			return skycon
		}
	}
	switch skycon {
	case "CLEAR_DAY", "CLEAR_NIGHT":
		return "晴"
	case "PARTLY_CLOUDY_DAY", "PARTLY_CLOUDY_NIGHT":
		return "多云"
	case "CLOUDY":
		return "阴"
	case "LIGHT_HAZE":
		return "轻雾"
	case "MODERATE_HAZE":
		return "中雾"
	case "HEAVY_HAZE":
		return "重雾"
	case "LIGHT_RAIN":
		return "小雨"
	case "MODERATE_RAIN":
		return "中雨"
	case "HEAVY_RAIN":
		return "大雨"
	case "STORM_RAIN":
		return "暴雨"
	case "FOG":
		return "雾"
	case "LIGHT_SNOW":
		return "小雪"
	case "MODERATE_SNOW":
		return "中雪"
	case "HEAVY_SNOW":
		return "大雪"
	case "STORM_SNOW":
		return "暴雪"
	case "DUST":
		return "浮尘"
	case "SAND":
		return "沙尘"
	case "WIND":
		return "大风"
	default:
		if skycon == "" {
			return "未知"
		}
		return skycon
	}
}

func weatherCodeName(code int, language string) string {
	if config.IsChinese(language) {
		switch code {
		case 0:
			return "晴"
		case 1, 2:
			return "少云"
		case 3:
			return "阴"
		case 45, 48:
			return "雾"
		case 51, 53, 55, 56, 57:
			return "毛毛雨"
		case 61, 63, 65, 66, 67:
			return "雨"
		case 71, 73, 75, 77:
			return "雪"
		case 80, 81, 82:
			return "阵雨"
		case 85, 86:
			return "阵雪"
		case 95, 96, 99:
			return "雷暴"
		default:
			return fmt.Sprintf("代码 %d", code)
		}
	}
	switch code {
	case 0:
		return "Clear"
	case 1, 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Fog"
	case 51, 53, 55, 56, 57:
		return "Drizzle"
	case 61, 63, 65, 66, 67:
		return "Rain"
	case 71, 73, 75, 77:
		return "Snow"
	case 80, 81, 82:
		return "Showers"
	case 85, 86:
		return "Snow showers"
	case 95, 96, 99:
		return "Thunderstorm"
	default:
		return fmt.Sprintf("Code %d", code)
	}
}
