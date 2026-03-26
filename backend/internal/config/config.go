package config

import "github.com/spf13/viper"

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	AI         AIConfig         `mapstructure:"ai"`
	Log        LogConfig        `mapstructure:"log"`
	Connectors ConnectorsConfig `mapstructure:"connectors"`
}

type ConnectorsConfig struct {
	Hevy      HevyConfig      `mapstructure:"hevy"`
	Strava    StravaConfig    `mapstructure:"strava"`
	Zenmoney  ZenmoneyConfig  `mapstructure:"zenmoney"`
}

type ZenmoneyConfig struct {
	Token string `mapstructure:"token"`
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
	viper.SetDefault("connectors.strava.redirect_uri", "http://localhost:8080/api/v1/auth/strava/callback")

	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.env", "development")
	viper.SetDefault("database.max_conns", 10)
	viper.SetDefault("ai.provider", "ollama")
	viper.SetDefault("ai.model", "llama3.1:8b")
	viper.SetDefault("ai.base_url", "http://ollama:11434")
	viper.SetDefault("log.level", "debug")

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
