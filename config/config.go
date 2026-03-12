package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Config holds the entire application configuration.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Alerting AlertingConfig `mapstructure:"alerting"`
	Auth     AuthConfig     `mapstructure:"auth"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	Host     string `mapstructure:"host"`
	LogLevel string `mapstructure:"log_level"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"` // "sqlite" or "postgres"
	DSN    string `mapstructure:"dsn"`    // connection string or file path
}

// AlertingConfig holds all alert channel configurations.
type AlertingConfig struct {
	Email    EmailConfig    `mapstructure:"email"`
	Telegram TelegramConfig `mapstructure:"telegram"`
	Discord  DiscordConfig  `mapstructure:"discord"`
	Webhook  WebhookConfig  `mapstructure:"webhook"`
}

// EmailConfig holds SMTP email settings.
type EmailConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
	To       string `mapstructure:"to"`
}

// TelegramConfig holds Telegram Bot settings.
type TelegramConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Token   string `mapstructure:"token"`
	ChatID  string `mapstructure:"chat_id"`
}

// DiscordConfig holds Discord webhook settings.
type DiscordConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"webhook_url"`
}

// WebhookConfig holds custom webhook settings.
type WebhookConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	URL     string `mapstructure:"url"`
	Method  string `mapstructure:"method"` // GET or POST
	Headers string `mapstructure:"headers"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

var (
	cfg     *Config
	cfgOnce sync.Once
	mu      sync.RWMutex
)

// Load reads the configuration from file and environment variables.
func Load(configPath string) (*Config, error) {
	var loadErr error

	cfgOnce.Do(func() {
		initLogger("info")

		v := viper.New()
		v.SetConfigType("yaml")

		if configPath != "" {
			v.SetConfigFile(configPath)
		} else {
			v.SetConfigName("config")
			v.AddConfigPath(".")
			v.AddConfigPath("/app")
			v.AddConfigPath("$HOME/.vyanawatch")
		}

		// Set defaults
		setDefaults(v)

		// Read environment variables
		v.SetEnvPrefix("VYANAWATCH")
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()

		// Read config file
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				loadErr = fmt.Errorf("error reading config file: %w", err)
				return
			}
			log.Warn().Msg("No config file found, using defaults and environment variables")
		} else {
			log.Info().Str("file", v.ConfigFileUsed()).Msg("Loaded configuration")
		}

		// Unmarshal into struct
		c := &Config{}
		if err := v.Unmarshal(c); err != nil {
			loadErr = fmt.Errorf("error unmarshaling config: %w", err)
			return
		}

		// Override with environment variables for secrets
		overrideFromEnv(c)

		mu.Lock()
		cfg = c
		mu.Unlock()

		initLogger(c.Server.LogLevel)

		// Watch for config changes (hot-reload)
		v.OnConfigChange(func(e fsnotify.Event) {
			log.Info().Str("file", e.Name).Msg("Config file changed, reloading...")
			newCfg := &Config{}
			if err := v.Unmarshal(newCfg); err != nil {
				log.Error().Err(err).Msg("Failed to reload config")
				return
			}
			overrideFromEnv(newCfg)
			mu.Lock()
			cfg = newCfg
			mu.Unlock()
			initLogger(newCfg.Server.LogLevel)
			log.Info().Msg("Configuration reloaded successfully")
		})
		v.WatchConfig()
	})

	if loadErr != nil {
		return nil, loadErr
	}

	return Get(), nil
}

// Get returns the current configuration (thread-safe).
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return cfg
}

// setDefaults configures sensible default values.
func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.log_level", "info")

	// Database
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "./data/vyanawatch.db")

	// Alerting defaults (all disabled)
	v.SetDefault("alerting.email.enabled", false)
	v.SetDefault("alerting.email.port", 587)
	v.SetDefault("alerting.telegram.enabled", false)
	v.SetDefault("alerting.discord.enabled", false)
	v.SetDefault("alerting.webhook.enabled", false)
	v.SetDefault("alerting.webhook.method", "POST")

	// Auth
	v.SetDefault("auth.enabled", false)
}

// overrideFromEnv overrides sensitive config values from environment variables.
func overrideFromEnv(c *Config) {
	if v := os.Getenv("VYANAWATCH_DB_DRIVER"); v != "" {
		c.Database.Driver = v
	}
	if v := os.Getenv("VYANAWATCH_DB_DSN"); v != "" {
		c.Database.DSN = v
	}
	if v := os.Getenv("VYANAWATCH_SMTP_PASSWORD"); v != "" {
		c.Alerting.Email.Password = v
	}
	if v := os.Getenv("VYANAWATCH_TELEGRAM_TOKEN"); v != "" {
		c.Alerting.Telegram.Token = v
	}
	if v := os.Getenv("VYANAWATCH_DISCORD_WEBHOOK_URL"); v != "" {
		c.Alerting.Discord.WebhookURL = v
	}
	if v := os.Getenv("VYANAWATCH_AUTH_PASSWORD"); v != "" {
		c.Auth.Password = v
	}
}

// initLogger configures the global zerolog logger.
func initLogger(level string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	log.Logger = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Caller().
		Logger()
}
