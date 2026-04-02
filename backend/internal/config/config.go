package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL           string
	Port                  string
	JWTSecret             string
	AgentEnrollmentSecret string
	JWTExpireHours        int
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	env := os.Getenv("ENV")
	if env == "production" || env == "prod" {
		validateRequiredEnvVars()
	}

	dbURL := getEnvRequired("DB_URL")
	jwtSecret := getEnvRequired("JWT_SECRET")
	agentSecret := getEnvRequired("AGENT_ENROLLMENT_SECRET")

	log.Println("[CONFIG] Loading configuration...")
	log.Printf("[CONFIG] Database: %s", maskDBConnectionString(dbURL))

	return &Config{
		DatabaseURL:           dbURL,
		Port:                  getEnvOrDefault("PORT", "8080"),
		JWTSecret:             jwtSecret,
		AgentEnrollmentSecret: agentSecret,
		JWTExpireHours:        24,
		ReadTimeout:           15 * time.Second,
		WriteTimeout:          15 * time.Second,
		IdleTimeout:           60 * time.Second,
	}, nil
}

func validateRequiredEnvVars() {
	required := []string{"DB_URL", "JWT_SECRET", "AGENT_ENROLLMENT_SECRET"}
	missing := []string{}

	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		log.Fatalf("[FATAL] Missing required production environment variables: %v", missing)
	}
}

func getEnvRequired(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("[FATAL] Required environment variable %s is not set", key)
	}
	return value
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func maskDBConnectionString(conn string) string {
	if conn == "" {
		return "(empty)"
	}
	return "postgres://***:***@***:***/***?sslmode=***"
}
