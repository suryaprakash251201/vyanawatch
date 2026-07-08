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

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Alerting AlertingConfig `mapstructure:"alerting"`
	Auth     AuthConfig     `mapstructure:"auth"`
}

type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	Host     string `mapstructure:"host"`
	LogLevel string `mapstructure:"log_level"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

type AlertingConfig struct {
	Email    EmailConfig    `mapstructure:"email"`
	Telegram TelegramConfig `mapstructure:"telegram"`
	Discord  DiscordConfig  `mapstructure:"discord"`
	Webhook  WebhookConfig  `mapstructure:"webhook"`
}

type EmailConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
	To       string `mapstructure:"to"`
}

type TelegramConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Token   string `mapstructure:"token"`
	ChatID  string `mapstructure:"chat_id"`
}

type DiscordConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"webhook_url"`
}

type WebhookConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	URL     string `mapstructure:"url"`
	Method  string `mapstructure:"method"`
	Headers string `mapstructure:"headers"`
}

type AuthConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type Provider struct {
	mu     sync.RWMutex
	cfg    *Config
	viper  *viper.Viper
	onChange []func(*Config)
}

func NewProvider(configPath string) (*Provider, error) {
	p := &Provider{}

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

	setDefaults(v)

	v.SetEnvPrefix("VYANAWATCH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		log.Warn().Msg("No config file found, using defaults and environment variables")
	} else {
		log.Info().Str("file", v.ConfigFileUsed()).Msg("Loaded configuration")
	}

	c := &Config{}
	if err := v.Unmarshal(c); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	overrideFromEnv(c)

	p.cfg = c
	p.viper = v

	initLogger(c.Server.LogLevel)

	v.OnConfigChange(func(e fsnotify.Event) {
		log.Info().Str("file", e.Name).Msg("Config file changed, reloading...")
		newCfg := &Config{}
		if err := v.Unmarshal(newCfg); err != nil {
			log.Error().Err(err).Msg("Failed to reload config")
			return
		}
		overrideFromEnv(newCfg)
		p.mu.Lock()
		p.cfg = newCfg
		p.mu.Unlock()
		initLogger(newCfg.Server.LogLevel)
		for _, fn := range p.onChange {
			fn(newCfg)
		}
		log.Info().Msg("Configuration reloaded successfully")
	})
	v.WatchConfig()

	return p, nil
}

func (p *Provider) Get() *Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

func (p *Provider) Update(cfg *Config) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
}

func (p *Provider) OnChange(fn func(*Config)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onChange = append(p.onChange, fn)
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.log_level", "info")

	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "./data/vyanawatch.db")

	v.SetDefault("alerting.email.enabled", false)
	v.SetDefault("alerting.email.port", 587)
	v.SetDefault("alerting.telegram.enabled", false)
	v.SetDefault("alerting.discord.enabled", false)
	v.SetDefault("alerting.webhook.enabled", false)
	v.SetDefault("alerting.webhook.method", "POST")

	v.SetDefault("auth.enabled", false)
}

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
