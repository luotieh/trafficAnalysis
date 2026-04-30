package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                    string
	StoreBackend            string
	DatabaseURL             string
	AutoMigrate             bool
	InternalAPIKey          string
	FlowShadowBaseURL       string
	FlowShadowAPIKey        string
	DeepSOCBaseURL          string
	DeepSOCUsername         string
	DeepSOCPassword         string
	DeepSOCAPIKey           string
	LLMBaseURL              string
	LLMAPIKey               string
	LLMModel                string
	SyncBatchSize           int
	SyncLookbackSeconds     int
	SyncMaxRetries          int
	HTTPTimeout             time.Duration
	MQBackend               string
	RabbitMQURL             string
	RabbitMQExchange        string
	RabbitMQEventQueue      string
	RabbitMQConsumerEnabled bool
}

func Load() Config {
	return Config{
		Addr:                    get("APP_ADDR", ":9010"),
		StoreBackend:            strings.ToLower(get("STORE_BACKEND", "memory")),
		DatabaseURL:             get("DATABASE_URL", ""),
		AutoMigrate:             getBool("AUTO_MIGRATE", true),
		InternalAPIKey:          get("INTERNAL_API_KEY", "change-me-internal-key"),
		FlowShadowBaseURL:       get("FLOWSHADOW_BASE_URL", ""),
		FlowShadowAPIKey:        get("FLOWSHADOW_API_KEY", ""),
		DeepSOCBaseURL:          get("DEEPSOC_BASE_URL", ""),
		DeepSOCUsername:         get("DEEPSOC_USERNAME", "admin"),
		DeepSOCPassword:         get("DEEPSOC_PASSWORD", "admin"),
		DeepSOCAPIKey:           get("DEEPSOC_API_KEY", ""),
		LLMBaseURL:              get("LLM_BASE_URL", ""),
		LLMAPIKey:               get("LLM_API_KEY", ""),
		LLMModel:                get("LLM_MODEL", "deepseek-chat"),
		SyncBatchSize:           getInt("SYNC_BATCH_SIZE", 200),
		SyncLookbackSeconds:     getInt("SYNC_LOOKBACK_SECONDS", 600),
		SyncMaxRetries:          getInt("SYNC_MAX_RETRIES", 5),
		HTTPTimeout:             time.Duration(getInt("HTTP_TIMEOUT_SECONDS", 15)) * time.Second,
		MQBackend:               strings.ToLower(get("MQ_BACKEND", "none")),
		RabbitMQURL:             get("RABBITMQ_URL", "amqp://traffic:traffic@127.0.0.1:5672/"),
		RabbitMQExchange:        get("RABBITMQ_EXCHANGE", "traffic.events"),
		RabbitMQEventQueue:      get("RABBITMQ_EVENT_QUEUE", "traffic.events.default"),
		RabbitMQConsumerEnabled: getBool("RABBITMQ_CONSUMER_ENABLED", true),
	}
}

func get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
