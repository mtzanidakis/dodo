package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	DatabasePath      string
	Listen            string
	EncryptionKey     []byte
	DefaultLocale     string
	DefaultTimezone   string
	SchedulerInterval time.Duration
	LogLevel          string
}

func Load() (Config, error) {
	c := Config{
		DatabasePath:      env("DODO_DATABASE_PATH", "/data/dodo.sqlite"),
		Listen:            env("DODO_LISTEN", ":8080"),
		DefaultLocale:     env("DODO_DEFAULT_LOCALE", "en"),
		DefaultTimezone:   env("DODO_DEFAULT_TIMEZONE", "Europe/Athens"),
		SchedulerInterval: envDuration("DODO_SCHEDULER_INTERVAL", time.Minute),
		LogLevel:          env("DODO_LOG_LEVEL", "info"),
	}

	if c.DatabasePath == "" {
		return Config{}, errors.New("DODO_DATABASE_PATH must not be empty")
	}

	if err := validateLocale(c.DefaultLocale); err != nil {
		return Config{}, err
	}

	if _, err := time.LoadLocation(c.DefaultTimezone); err != nil {
		return Config{}, fmt.Errorf("invalid DODO_DEFAULT_TIMEZONE %q: %w", c.DefaultTimezone, err)
	}

	if c.SchedulerInterval < time.Minute {
		return Config{}, fmt.Errorf("DODO_SCHEDULER_INTERVAL %s is below the 1m minimum", c.SchedulerInterval)
	}

	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("invalid DODO_LOG_LEVEL %q (want debug|info|warn|error)", c.LogLevel)
	}

	key := os.Getenv("DODO_ENCRYPTION_KEY")
	if key == "" {
		return Config{}, errors.New("DODO_ENCRYPTION_KEY is required (generate with: openssl rand -base64 32)")
	}
	decoded, err := decodeEncryptionKey(key)
	if err != nil {
		return Config{}, err
	}
	c.EncryptionKey = decoded

	return c, nil
}

func LoadAdmin() (Config, error) {
	c := Config{
		DatabasePath:      env("DODO_DATABASE_PATH", "/data/dodo.sqlite"),
		Listen:            env("DODO_LISTEN", ":8080"),
		DefaultLocale:     env("DODO_DEFAULT_LOCALE", "en"),
		DefaultTimezone:   env("DODO_DEFAULT_TIMEZONE", "Europe/Athens"),
		SchedulerInterval: envDuration("DODO_SCHEDULER_INTERVAL", time.Minute),
		LogLevel:          env("DODO_LOG_LEVEL", "info"),
	}
	if c.DatabasePath == "" {
		return Config{}, errors.New("DODO_DATABASE_PATH must not be empty")
	}
	if _, err := time.LoadLocation(c.DefaultTimezone); err != nil {
		return Config{}, fmt.Errorf("invalid DODO_DEFAULT_TIMEZONE %q: %w", c.DefaultTimezone, err)
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("invalid DODO_LOG_LEVEL %q", c.LogLevel)
	}
	return c, nil
}

func decodeEncryptionKey(raw string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, fmt.Errorf("DODO_ENCRYPTION_KEY must be a 32-byte base64 key (got %d decoded bytes)", len(b))
}

func validateLocale(l string) error {
	switch l {
	case "en", "el":
		return nil
	default:
		return fmt.Errorf("invalid locale %q (want en|el)", l)
	}
}

func env(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

var _ = strings.ToLower
