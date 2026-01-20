package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	ServerPort         string
	DBTimezone         string
	AuthValidateURL    string
	ProductServiceURL  string
	RabbitMQURL        string
	RabbitMQQueue      string
	RabbitMQExchange   string
	RabbitMQRoutingKey string
	SalesQueue         string
	SalesExchange      string
	SalesRoutingKey    string
	CustomerServiceURL string
}

func LoadConfig() (*Config, error) {
	// Load .env file if it exists (ok to ignore error in prod if env vars are set)
	_ = godotenv.Load()

	cfg := &Config{
		DBHost:             os.Getenv("DB_HOST"),
		DBPort:             os.Getenv("DB_PORT"),
		DBUser:             os.Getenv("DB_USER"),
		DBPassword:         os.Getenv("DB_PASSWORD"),
		DBName:             os.Getenv("DB_NAME"),
		ServerPort:         os.Getenv("SERVER_PORT"),
		DBTimezone:         os.Getenv("DB_TIMEZONE"), // Defaults to empty
		AuthValidateURL:    os.Getenv("AUTH_VALIDATE_URL"),
		ProductServiceURL:  os.Getenv("PRODUCT_SERVICE_URL"),
		RabbitMQURL:        os.Getenv("RABBITMQ_URL"),
		RabbitMQQueue:      os.Getenv("RABBITMQ_QUEUE"),
		RabbitMQExchange:   os.Getenv("RABBITMQ_EXCHANGE"),
		RabbitMQRoutingKey: os.Getenv("RABBITMQ_ROUTING_KEY"),
		SalesQueue:         os.Getenv("SALES_QUEUE"),
		SalesExchange:      os.Getenv("SALES_EXCHANGE"),
		SalesRoutingKey:    os.Getenv("SALES_ROUTING_KEY"),
		CustomerServiceURL: os.Getenv("CUSTOMER_SERVICE_URL"),
	}

	if cfg.DBHost == "" || cfg.DBUser == "" {
		return nil, fmt.Errorf("missing required environment variables")
	}

	if cfg.DBTimezone == "" {
		cfg.DBTimezone = "Asia/Jakarta"
	}

	// Set default values for sales consumer if not provided
	if cfg.SalesQueue == "" {
		cfg.SalesQueue = "sales.queue"
	}
	if cfg.SalesExchange == "" {
		cfg.SalesExchange = "sales.exchange"
	}
	if cfg.SalesRoutingKey == "" {
		cfg.SalesRoutingKey = "transaction.sales"
	}

	return cfg, nil
}
