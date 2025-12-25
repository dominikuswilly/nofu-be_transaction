package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	ServerPort string
	DBTimezone string
}

func LoadConfig() (*Config, error) {
	// Load .env file if it exists (ok to ignore error in prod if env vars are set)
	_ = godotenv.Load()

	cfg := &Config{
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		ServerPort: os.Getenv("SERVER_PORT"),
		DBTimezone: os.Getenv("DB_TIMEZONE"), // Defaults to empty
	}

	if cfg.DBHost == "" || cfg.DBUser == "" {
		return nil, fmt.Errorf("missing required environment variables")
	}

	if cfg.DBTimezone == "" {
		cfg.DBTimezone = "Asia/Jakarta"
	}

	return cfg, nil
}
