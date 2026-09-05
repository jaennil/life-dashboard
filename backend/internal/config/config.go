package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Auth       AuthConfig       `mapstructure:"auth"`
	AI         AIConfig         `mapstructure:"ai"`
	Log        LogConfig        `mapstructure:"log"`
	Sentry     SentryConfig     `mapstructure:"sentry"`
	WebPush    WebPushConfig    `mapstructure:"web_push"`
	Telegram   TelegramConfig   `mapstructure:"telegram"`
	Connectors ConnectorsConfig `mapstructure:"connectors"`
	Weather    WeatherConfig    `mapstructure:"weather"`
	Unleash    UnleashConfig    `mapstructure:"unleash"`
}

type WebPushConfig struct {
	PublicKey  string `mapstructure:"public_key"`
	PrivateKey string `mapstructure:"private_key"`
	Subscriber string `mapstructure:"subscriber"`
}

// TelegramConfig carries the one bot the whole instance talks through. An empty
// token disables every Telegram feature rather than failing at send time.
type TelegramConfig struct {
	BotToken string `mapstructure:"bot_token"`
	// APIBase points the bot at something other than Telegram itself, which is
	// how the delivery path is exercised locally without a real bot.
	APIBase string `mapstructure:"api_base"`
}

type UnleashConfig struct {
	URL      string `mapstructure:"url"`
	APIToken string `mapstructure:"api_token"`
	AppName  string `mapstructure:"app_name"`
}

type ConnectorsConfig struct {
	Hevy           HevyConfig           `mapstructure:"hevy"`
	Strava         StravaConfig         `mapstructure:"strava"`
	Zenmoney       ZenmoneyConfig       `mapstructure:"zenmoney"`
	MFP            MFPConfig            `mapstructure:"mfp"`
	FatSecret      FatSecretConfig      `mapstructure:"fatsecret"`
	GoogleCalendar GoogleCalendarConfig `mapstructure:"google_calendar"`
	Notion         NotionConfig         `mapstructure:"notion"`
	Todoist        TodoistConfig        `mapstructure:"todoist"`
	Xiaomi         XiaomiConfig         `mapstructure:"xiaomi"`
}

// XiaomiConfig only carries routing. Credentials are per-user and live in
// oauth_tokens, because Xiaomi rotates the passToken on every login.
type XiaomiConfig struct {
	Region string `mapstructure:"region"`
	Model  string `mapstructure:"model"`
}

type TodoistConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
}

type NotionConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
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
	// ReasoningEffort is passed through to the upstream as reasoning_effort.
	// Empty means the field is omitted entirely, because a provider that does
	// not know it rejects the whole request rather than ignoring it.
	ReasoningEffort string `mapstructure:"reasoning_effort"`
	// RequestTimeout has to cover thinking, not just writing: a reasoning model
	// on a high effort setting can spend minutes before it emits a first token.
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	// ParseModel and ParseReasoningEffort override Model and ReasoningEffort for
	// extraction work such as turning a dictated phrase into exercises. Measured
	// on the same phrase: the checkup model spent 43s and 2 RUB thinking, the fast
	// one 5s and 0.09 RUB with no thinking at all, and the answer was as good once
	// the prompt spelled out the set-count rule.
	ParseModel           string `mapstructure:"parse_model"`
	ParseReasoningEffort string `mapstructure:"parse_reasoning_effort"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

type SentryConfig struct {
	BackendDSN       string  `mapstructure:"backend_dsn"`
	Environment      string  `mapstructure:"environment"`
	Release          string  `mapstructure:"release"`
	TracesSampleRate float64 `mapstructure:"traces_sample_rate"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/app")

	viper.BindEnv("database.url", "DATABASE_URL")
	viper.BindEnv("ai.api_key", "AI_API_KEY")
	viper.BindEnv("server.env", "SERVER_ENV")
	viper.BindEnv("sentry.backend_dsn", "SENTRY_BACKEND_DSN")
	viper.BindEnv("sentry.environment", "SENTRY_ENVIRONMENT")
	viper.BindEnv("sentry.release", "SENTRY_RELEASE")
	viper.BindEnv("sentry.traces_sample_rate", "SENTRY_BACKEND_TRACES_SAMPLE_RATE")
	viper.BindEnv("telegram.bot_token", "TELEGRAM_BOT_TOKEN")
	viper.BindEnv("telegram.api_base", "TELEGRAM_API_BASE")
	viper.BindEnv("web_push.public_key", "WEB_PUSH_PUBLIC_KEY")
	viper.BindEnv("web_push.private_key", "WEB_PUSH_PRIVATE_KEY")
	viper.BindEnv("web_push.subscriber", "WEB_PUSH_SUBSCRIBER")
	viper.SetDefault("web_push.subscriber", "mailto:admin@dubrovskih.ru")
	viper.BindEnv("connectors.hevy.api_key", "HEVY_API_KEY")
	viper.BindEnv("connectors.xiaomi.region", "XIAOMI_REGION")
	viper.BindEnv("connectors.xiaomi.model", "XIAOMI_MODEL")
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
	viper.BindEnv("connectors.notion.client_id", "NOTION_CLIENT_ID")
	viper.BindEnv("connectors.notion.client_secret", "NOTION_CLIENT_SECRET")
	viper.BindEnv("connectors.notion.redirect_uri", "NOTION_REDIRECT_URI")
	viper.SetDefault("connectors.notion.redirect_uri", "http://localhost:8080/api/v1/auth/notion/callback")
	viper.BindEnv("connectors.todoist.client_id", "TODOIST_CLIENT_ID")
	viper.BindEnv("connectors.todoist.client_secret", "TODOIST_CLIENT_SECRET")
	viper.BindEnv("connectors.todoist.redirect_uri", "TODOIST_REDIRECT_URI")
	viper.SetDefault("connectors.todoist.redirect_uri", "http://localhost:8080/api/v1/auth/todoist/callback")
	viper.BindEnv("unleash.url", "UNLEASH_URL")
	viper.BindEnv("unleash.api_token", "UNLEASH_API_TOKEN")
	viper.SetDefault("unleash.url", "http://unleash.unleash.svc.cluster.local:4242/api")
	viper.SetDefault("unleash.app_name", "life-dashboard")

	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.env", "development")
	viper.SetDefault("sentry.environment", "")
	viper.SetDefault("sentry.traces_sample_rate", 0)
	viper.SetDefault("database.max_conns", 10)
	viper.BindEnv("ai.base_url", "AI_BASE_URL")
	viper.BindEnv("ai.model", "AI_MODEL")
	viper.BindEnv("ai.reasoning_effort", "AI_REASONING_EFFORT")
	viper.BindEnv("ai.parse_model", "AI_PARSE_MODEL")
	viper.BindEnv("ai.parse_reasoning_effort", "AI_PARSE_REASONING_EFFORT")
	viper.BindEnv("ai.request_timeout", "AI_REQUEST_TIMEOUT")
	viper.SetDefault("ai.provider", "claude-code-api")
	viper.SetDefault("ai.request_timeout", 10*time.Minute)
	viper.SetDefault("ai.model", "claude-opus-4-5-20251101")
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
