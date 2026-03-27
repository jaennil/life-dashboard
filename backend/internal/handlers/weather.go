package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type WeatherHandler struct {
	lat    float64
	lon    float64
	city   string
	client *http.Client
	logger zerolog.Logger
}

func NewWeather(lat, lon float64, city string, logger zerolog.Logger) *WeatherHandler {
	return &WeatherHandler{
		lat:    lat,
		lon:    lon,
		city:   city,
		client: &http.Client{Timeout: 10 * time.Second},
		logger: logger.With().Str("handler", "weather").Logger(),
	}
}

type WeatherResponse struct {
	City        string        `json:"city"`
	Temp        float64       `json:"temp"`
	FeelsLike   float64       `json:"feels_like"`
	WeatherCode int           `json:"weather_code"`
	Description string        `json:"description"`
	Humidity    int           `json:"humidity"`
	WindSpeed   float64       `json:"wind_speed"`
	Hourly      []HourlyPoint `json:"hourly"`
	Daily       []DailyPoint  `json:"daily"`
}

type HourlyPoint struct {
	Time        string  `json:"time"`
	Temp        float64 `json:"temp"`
	WeatherCode int     `json:"weather_code"`
}

type DailyPoint struct {
	Date        string  `json:"date"`
	TempMax     float64 `json:"temp_max"`
	TempMin     float64 `json:"temp_min"`
	WeatherCode int     `json:"weather_code"`
}

func wmoDescription(code int) string {
	switch {
	case code == 0:
		return "Ясно"
	case code == 1:
		return "Преимущественно ясно"
	case code == 2:
		return "Переменная облачность"
	case code == 3:
		return "Пасмурно"
	case code == 45 || code == 48:
		return "Туман"
	case code == 51 || code == 53 || code == 55:
		return "Морось"
	case code == 61 || code == 63 || code == 65:
		return "Дождь"
	case code == 71 || code == 73 || code == 75:
		return "Снег"
	case code == 77:
		return "Ледяная крупа"
	case code == 80 || code == 81 || code == 82:
		return "Ливень"
	case code == 85 || code == 86:
		return "Снежный ливень"
	case code == 95:
		return "Гроза"
	case code == 96 || code == 99:
		return "Гроза с градом"
	default:
		return "Неизвестно"
	}
}

func (h *WeatherHandler) Fetch(lat, lon float64, city string) (*WeatherResponse, error) {
	if lat == 0 && lon == 0 {
		lat, lon = h.lat, h.lon
	}
	if city == "" {
		city = h.city
	}
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&current=temperature_2m,apparent_temperature,weather_code,wind_speed_10m,relative_humidity_2m"+
			"&hourly=temperature_2m,weather_code"+
			"&daily=temperature_2m_max,temperature_2m_min,weather_code"+
			"&timezone=auto&forecast_days=5",
		lat, lon,
	)

	resp, err := h.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch open-meteo: %w", err)
	}
	defer resp.Body.Close()

	var raw struct {
		Current struct {
			Temperature  float64 `json:"temperature_2m"`
			ApparentTemp float64 `json:"apparent_temperature"`
			WeatherCode  int     `json:"weather_code"`
			WindSpeed    float64 `json:"wind_speed_10m"`
			Humidity     int     `json:"relative_humidity_2m"`
		} `json:"current"`
		Hourly struct {
			Time        []string  `json:"time"`
			Temperature []float64 `json:"temperature_2m"`
			WeatherCode []int     `json:"weather_code"`
		} `json:"hourly"`
		Daily struct {
			Time        []string  `json:"time"`
			TempMax     []float64 `json:"temperature_2m_max"`
			TempMin     []float64 `json:"temperature_2m_min"`
			WeatherCode []int     `json:"weather_code"`
		} `json:"daily"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode open-meteo: %w", err)
	}

	now := time.Now()
	currentHour := now.Format("2006-01-02T15:00")

	var hourly []HourlyPoint
	for i, t := range raw.Hourly.Time {
		if t < currentHour {
			continue
		}
		if len(hourly) >= 24 {
			break
		}
		hourly = append(hourly, HourlyPoint{
			Time:        t,
			Temp:        raw.Hourly.Temperature[i],
			WeatherCode: raw.Hourly.WeatherCode[i],
		})
	}

	var daily []DailyPoint
	for i, t := range raw.Daily.Time {
		daily = append(daily, DailyPoint{
			Date:        t,
			TempMax:     raw.Daily.TempMax[i],
			TempMin:     raw.Daily.TempMin[i],
			WeatherCode: raw.Daily.WeatherCode[i],
		})
	}

	result := &WeatherResponse{
		City:        city,
		Temp:        raw.Current.Temperature,
		FeelsLike:   raw.Current.ApparentTemp,
		WeatherCode: raw.Current.WeatherCode,
		Description: wmoDescription(raw.Current.WeatherCode),
		Humidity:    raw.Current.Humidity,
		WindSpeed:   raw.Current.WindSpeed,
		Hourly:      hourly,
		Daily:       daily,
	}

	h.logger.Info().Float64("temp", result.Temp).Int("code", result.WeatherCode).Msg("weather fetched")
	return result, nil
}

func (h *WeatherHandler) GetWeather(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var lat, lon float64
	fmt.Sscanf(q.Get("lat"), "%f", &lat)
	fmt.Sscanf(q.Get("lon"), "%f", &lon)
	city := q.Get("city")

	result, err := h.Fetch(lat, lon, city)
	if err != nil {
		h.logger.Error().Err(err).Msg("get weather")
		http.Error(w, "weather unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
