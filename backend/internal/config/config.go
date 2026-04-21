package config

import (
	"fmt"
	"os"
	"reflect"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Turnstile    TurnstileConfig    `yaml:"turnstile"`
	Mailgun      MailgunConfig      `yaml:"mailgun"`
	Contact      ContactConfig      `yaml:"contact"`
	Verification VerificationConfig `yaml:"verification"`
	OpenAI       OpenAIConfig       `yaml:"openai"`
	CSVLog       CSVLogConfig       `yaml:"csvlog"`
	RateLimit    RateLimitConfig    `yaml:"rate_limit"`
}

type ServerConfig struct {
	Port           int      `yaml:"port"`
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type TurnstileConfig struct {
	SecretKey string `yaml:"secret_key"`
}

type MailgunConfig struct {
	Domain string `yaml:"domain"`
	APIKey string `yaml:"api_key"`
	Sender string `yaml:"sender"`
}

type ContactConfig struct {
	Recipient string `yaml:"recipient"`
}

type VerificationConfig struct {
	CodeTTLMinutes int `yaml:"code_ttl_minutes"`
	MaxAttempts    int `yaml:"max_attempts"`
}

type OpenAIConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

type CSVLogConfig struct {
	FilePath string `yaml:"file_path"`
}

type RateLimitConfig struct {
	MaxPerIPBrowser int `yaml:"max_per_ip_browser"`
	MaxPerIP        int `yaml:"max_per_ip"`
	WindowMinutes   int `yaml:"window_minutes"`
}

func Load(path string) (*Config, error) {
	// Load .env file if present (from working directory)
	loadDotEnv(".env")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Substitute {{env:VAR}} in all string fields
	walkAndSubstStrings(reflect.ValueOf(&cfg))

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Verification.CodeTTLMinutes == 0 {
		cfg.Verification.CodeTTLMinutes = 10
	}
	if cfg.Verification.MaxAttempts == 0 {
		cfg.Verification.MaxAttempts = 5
	}
	if cfg.OpenAI.Model == "" {
		cfg.OpenAI.Model = "gpt-5.4-nano"
	}
	if cfg.CSVLog.FilePath == "" {
		cfg.CSVLog.FilePath = "submissions.csv"
	}
	if cfg.RateLimit.MaxPerIPBrowser == 0 {
		cfg.RateLimit.MaxPerIPBrowser = 10
	}
	if cfg.RateLimit.MaxPerIP == 0 {
		cfg.RateLimit.MaxPerIP = 50
	}
	if cfg.RateLimit.WindowMinutes == 0 {
		cfg.RateLimit.WindowMinutes = 60
	}

	return &cfg, nil
}
