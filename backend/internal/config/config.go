package config

import "github.com/spf13/viper"

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Auth       AuthConfig       `mapstructure:"auth"`
	AI         AIConfig         `mapstructure:"ai"`
	Log        LogConfig        `mapstructure:"log"`
	Connectors ConnectorsConfig `mapstructure:"connectors"`
	Weather    WeatherConfig    `mapstructure:"weather"`
}

type ConnectorsConfig struct {
	Hevy           HevyConfig           `mapstructure:"hevy"`
	Strava         StravaConfig         `mapstructure:"strava"`
	Zenmoney       ZenmoneyConfig       `mapstructure:"zenmoney"`
	MFP            MFPConfig            `mapstructure:"mfp"`
	FatSecret      FatSecretConfig      `mapstructure:"fatsecret"`
	GoogleCalendar GoogleCalendarConfig `mapstructure:"google_calendar"`
}

type GoogleCalendarConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
}

type FatSecretConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
}

type MFPConfig struct {
	Username      string `mapstructure:"username"`
	Password      string `mapstructure:"password"`
	SessionCookie string `mapstructure:"session_cookie"`
	AccessToken   string `mapstructure:"access_token"`
	UserID        string `mapstructure:"user_id"`
}

type WeatherConfig struct {
	Lat  float64 `mapstructure:"lat"`
	Lon  float64 `mapstructure:"lon"`
	City string  `mapstructure:"city"`
}

type ZenmoneyConfig struct {
	Token        string `mapstructure:"token"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
}

type HevyConfig struct {
	APIKey string `mapstructure:"api_key"`
}

type StravaConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Env  string `mapstructure:"env"`
}

type DatabaseConfig struct {
	URL      string `mapstructure:"url"`
	MaxConns int32  `mapstructure:"max_conns"`
}

type AuthConfig struct {
	JWTSecret string `mapstructure:"jwt_secret"`
}

type AIConfig struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
	BaseURL  string `mapstructure:"base_url"`
	APIKey   string `mapstructure:"api_key"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/app")

	viper.BindEnv("database.url", "DATABASE_URL")
	viper.BindEnv("ai.api_key", "AI_API_KEY")
	viper.BindEnv("server.env", "SERVER_ENV")
	viper.BindEnv("connectors.hevy.api_key", "HEVY_API_KEY")
	viper.BindEnv("connectors.strava.client_id", "STRAVA_CLIENT_ID")
	viper.BindEnv("connectors.strava.client_secret", "STRAVA_CLIENT_SECRET")
	viper.BindEnv("connectors.strava.redirect_uri", "STRAVA_REDIRECT_URI")
	viper.BindEnv("connectors.zenmoney.token", "ZENMONEY_TOKEN")
	viper.BindEnv("connectors.zenmoney.client_id", "ZENMONEY_CLIENT_ID")
	viper.BindEnv("connectors.zenmoney.client_secret", "ZENMONEY_CLIENT_SECRET")
	viper.BindEnv("connectors.zenmoney.redirect_uri", "ZENMONEY_REDIRECT_URI")
	viper.SetDefault("connectors.strava.redirect_uri", "http://localhost:8080/api/v1/auth/strava/callback")
	viper.SetDefault("connectors.zenmoney.redirect_uri", "http://localhost:8080/api/v1/auth/zenmoney/callback")
	viper.BindEnv("connectors.google_calendar.client_id", "GOOGLE_CLIENT_ID")
	viper.BindEnv("connectors.google_calendar.client_secret", "GOOGLE_CLIENT_SECRET")
	viper.BindEnv("connectors.google_calendar.redirect_uri", "GOOGLE_REDIRECT_URI")
	viper.SetDefault("connectors.google_calendar.redirect_uri", "http://localhost:8080/api/v1/auth/google/callback")

	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.env", "development")
	viper.SetDefault("database.max_conns", 10)
	viper.BindEnv("ai.base_url", "AI_BASE_URL")
	viper.BindEnv("ai.model", "AI_MODEL")
	viper.SetDefault("ai.provider", "claude-code-api")
	viper.SetDefault("ai.model", "claude-sonnet-4-5-20250929")
	viper.SetDefault("ai.base_url", "http://host.docker.internal:8000")
	viper.SetDefault("log.level", "debug")

	viper.BindEnv("connectors.fatsecret.client_id", "FATSECRET_CLIENT_ID")
	viper.BindEnv("connectors.fatsecret.client_secret", "FATSECRET_CLIENT_SECRET")
	viper.BindEnv("connectors.fatsecret.redirect_uri", "FATSECRET_REDIRECT_URI")
	viper.SetDefault("connectors.fatsecret.redirect_uri", "http://localhost:8080/api/v1/auth/fatsecret/callback")

	viper.BindEnv("auth.jwt_secret", "JWT_SECRET")
	viper.SetDefault("auth.jwt_secret", "change-me-in-production-please")

	viper.BindEnv("connectors.mfp.username", "MFP_USERNAME")
	viper.BindEnv("connectors.mfp.password", "MFP_PASSWORD")
	viper.BindEnv("connectors.mfp.session_cookie", "MFP_SESSION_COOKIE")
	viper.BindEnv("connectors.mfp.access_token", "MFP_ACCESS_TOKEN")
	viper.BindEnv("connectors.mfp.user_id", "MFP_USER_ID")

	viper.BindEnv("weather.lat", "WEATHER_LAT")
	viper.BindEnv("weather.lon", "WEATHER_LON")
	viper.BindEnv("weather.city", "WEATHER_CITY")
	viper.SetDefault("weather.lat", 55.7522)
	viper.SetDefault("weather.lon", 37.6156)
	viper.SetDefault("weather.city", "Москва")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
