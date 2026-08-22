package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Addr        string
	DatabaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:        strings.TrimSpace(os.Getenv("WERSTICS_VERIFY_ADDR")),
		DatabaseURL: strings.TrimSpace(os.Getenv("WERSTICS_VERIFY_DATABASE_URL")),
	}

	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf(
			"WERSTICS_VERIFY_DATABASE_URL is required",
		)
	}

	if err := validateDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid database URL: %w", err)
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf(
			"database URL must use postgres or postgresql scheme",
		)
	}

	if u.Host == "" {
		return fmt.Errorf("database URL must include a host")
	}

	if u.User == nil || u.User.Username() == "" {
		return fmt.Errorf("database URL must include a username")
	}

	return nil
}
