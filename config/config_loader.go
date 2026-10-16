package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds runtime configuration. Values are read from an optional YAML
// file first and then overridden by environment variables, so the container
// can be configured purely through env vars.
type Config struct {
	Port                  string   `yaml:"port"`
	DatabaseURL           string   `yaml:"database_url"`
	JWTSecret             string   `yaml:"jwt_secret"`
	OpenAIAPIKey          string   `yaml:"openai_api_key"`
	OpenAIBaseURL         string   `yaml:"openai_base_url"`
	OpenAIEvalModel       string   `yaml:"openai_eval_model"`
	OpenAITranscribeModel string   `yaml:"openai_transcribe_model"`
	AppTimezone           string   `yaml:"app_timezone"`
	CORSOrigins           []string `yaml:"cors_origins"`
	SupabaseURL           string   `yaml:"supabase_url"`
	SupabaseServiceKey    string   `yaml:"supabase_service_key"`
	SupabaseBucket        string   `yaml:"supabase_bucket"`
}

func New(path string) (*Config, error) {
	cfg, err := load(path)
	if err != nil {
		return nil, err
	}
	cfg.applyEnv()
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) GetDatabaseURL() string {
	return c.DatabaseURL
}

// Location returns the timezone used to bucket activity into days.
func (c *Config) Location() *time.Location {
	loc, err := time.LoadLocation(c.AppTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func load(path string) (*Config, error) {
	var cfg Config
	if path == "" {
		return &cfg, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyEnv() {
	setFromEnv(&c.Port, "PORT")
	setFromEnv(&c.DatabaseURL, "DATABASE_URL")
	setFromEnv(&c.JWTSecret, "JWT_SECRET")
	setFromEnv(&c.OpenAIAPIKey, "OPENAI_API_KEY")
	setFromEnv(&c.OpenAIBaseURL, "OPENAI_BASE_URL")
	setFromEnv(&c.OpenAIEvalModel, "OPENAI_EVAL_MODEL")
	setFromEnv(&c.OpenAITranscribeModel, "OPENAI_TRANSCRIBE_MODEL")
	setFromEnv(&c.AppTimezone, "APP_TIMEZONE")
	setFromEnv(&c.SupabaseURL, "SUPABASE_URL")
	setFromEnv(&c.SupabaseServiceKey, "SUPABASE_SERVICE_KEY")
	setFromEnv(&c.SupabaseBucket, "SUPABASE_BUCKET")
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		c.CORSOrigins = nil
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.CORSOrigins = append(c.CORSOrigins, o)
			}
		}
	}
}

func (c *Config) applyDefaults() {
	if c.Port == "" {
		c.Port = "8080"
	}
	if c.OpenAIBaseURL == "" {
		c.OpenAIBaseURL = "https://api.openai.com/v1"
	}
	if c.OpenAIEvalModel == "" {
		c.OpenAIEvalModel = "gpt-4o-mini"
	}
	if c.OpenAITranscribeModel == "" {
		c.OpenAITranscribeModel = "gpt-4o-mini-transcribe"
	}
	if c.AppTimezone == "" {
		c.AppTimezone = "Asia/Jakarta"
	}
	if len(c.CORSOrigins) == 0 {
		c.CORSOrigins = []string{"https://docs.hearable.nighttealabs.tech", "http://localhost:3000", "http://localhost:5173", "https://hearable.nighttealabs.tech"}
	}
	if c.SupabaseBucket == "" {
		c.SupabaseBucket = "audio"
	}
}

func (c *Config) validate() error {
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "database_url / DATABASE_URL")
	}
	if c.JWTSecret == "" {
		missing = append(missing, "jwt_secret / JWT_SECRET")
	}
	if c.OpenAIAPIKey == "" {
		missing = append(missing, "openai_api_key / OPENAI_API_KEY")
	}
	if c.SupabaseURL == "" {
		missing = append(missing, "supabase_url / SUPABASE_URL")
	}
	if c.SupabaseServiceKey == "" {
		missing = append(missing, "supabase_service_key / SUPABASE_SERVICE_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("jwt_secret must be at least 32 characters")
	}
	if _, err := time.LoadLocation(c.AppTimezone); err != nil {
		return fmt.Errorf("invalid app_timezone %q: %w", c.AppTimezone, err)
	}
	return nil
}

func setFromEnv(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}
