package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config хранит всю конфигурацию сервиса. Значения приходят из окружения.
type Config struct {
	Env                  string
	LogLevel             string
	Port                 string
	JWTSecret            string
	DBPath               string
	OrchestratorHost     string
	OrchestratorPort     string
	AgentBootstrapKey    string
	AgentTokenTTL        time.Duration
	UserTokenTTL         time.Duration
	TaskPollInterval     time.Duration
	RequestTimeout       time.Duration
	MaxExpressionLen     int
	MaxTasksPerExpr      int
	MaxOperationTimeMS   int
	TimeAdditionMS       int
	TimeSubtractionMS    int
	TimeMultiplicationMS int
	TimeDivisionMS       int
	MaxRetries           int
	RetryBaseDelay       time.Duration
}

// Load читает конфигурацию из окружения и проверяет обязательные поля.
func Load() (*Config, error) {
	cfg := &Config{
		Env:                  envDefault("ENV", "dev"),
		LogLevel:             envDefault("LOG_LEVEL", "info"),
		Port:                 envDefault("PORT", "8080"),
		JWTSecret:            os.Getenv("JWT_SECRET"),
		DBPath:               envDefault("DB_PATH", "data/calculator.db"),
		OrchestratorHost:     envDefault("ORCHESTRATOR_HOST", "localhost"),
		OrchestratorPort:     envDefault("ORCHESTRATOR_PORT", "8080"),
		AgentBootstrapKey:    os.Getenv("AGENT_BOOTSTRAP_KEY"),
		AgentTokenTTL:        durationDefault("AGENT_TOKEN_TTL", 24*time.Hour),
		UserTokenTTL:         durationDefault("USER_TOKEN_TTL", 24*time.Hour),
		TaskPollInterval:     durationDefault("TASK_POLL_INTERVAL", 500*time.Millisecond),
		RequestTimeout:       durationDefault("REQUEST_TIMEOUT", 10*time.Second),
		MaxExpressionLen:     intDefault("MAX_EXPRESSION_LEN", 256),
		MaxTasksPerExpr:      intDefault("MAX_TASKS_PER_EXPR", 256),
		MaxOperationTimeMS:   intDefault("MAX_OPERATION_TIME_MS", 30000),
		TimeAdditionMS:       nonNegativeIntDefault("TIME_ADDITION_MS", 0),
		TimeSubtractionMS:    nonNegativeIntDefault("TIME_SUBTRACTION_MS", 0),
		TimeMultiplicationMS: nonNegativeIntDefault("TIME_MULTIPLICATIONS_MS", 0),
		TimeDivisionMS:       nonNegativeIntDefault("TIME_DIVISIONS_MS", 0),
		MaxRetries:           intDefault("MAX_RETRIES", 5),
		RetryBaseDelay:       durationDefault("RETRY_BASE_DELAY", 2*time.Second),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	secret := strings.TrimSpace(c.JWTSecret)
	if secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	for _, bad := range []string{
		"change-me", "super_secret_key", "insecure-default-secret",
		"replace-with-a-long-random-secret", "replace-with-another-long-random-key",
	} {
		if secret == bad {
			return fmt.Errorf("JWT_SECRET is a known placeholder")
		}
	}
	if len(secret) < 16 {
		return fmt.Errorf("JWT_SECRET must be at least 16 characters")
	}
	if c.Port == "" || c.OrchestratorPort == "" {
		return fmt.Errorf("PORT and ORCHESTRATOR_PORT must not be empty")
	}
	if c.MaxExpressionLen < 1 || c.MaxTasksPerExpr < 1 {
		return fmt.Errorf("expression and task limits must be positive")
	}
	if c.AgentBootstrapKey == "" {
		return fmt.Errorf("AGENT_BOOTSTRAP_KEY is required")
	}
	return nil
}

// BaseURL возвращает адрес оркестратора для агента.
func (c *Config) BaseURL() string {
	return "http://" + c.OrchestratorHost + ":" + c.OrchestratorPort
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func durationDefault(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

// nonNegativeIntDefault принимает 0 как осознанную настройку: время операции
// можно отключить, чтобы вычисления шли без искусственной задержки.
func nonNegativeIntDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}
