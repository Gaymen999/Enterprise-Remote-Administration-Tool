package config

import (
	"fmt"
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
		secrets, err := LoadProductionSecrets()
		if err != nil {
			log.Fatalf("[FATAL] Failed to load production secrets: %v", err)
		}

		return &Config{
			DatabaseURL:           secrets.DatabaseURL,
			JWTSecret:             secrets.JWTSecret,
			AgentEnrollmentSecret: secrets.AgentEnrollmentSecret,
			Port:                  getEnvOrDefault("PORT", "8080"),
			JWTExpireHours:        24,
			ReadTimeout:           15 * time.Second,
			WriteTimeout:          15 * time.Second,
			IdleTimeout:           60 * time.Second,
		}, nil
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

type ProductionSecrets struct {
	DatabaseURL           string
	JWTSecret             string
	AgentEnrollmentSecret string
}

func LoadProductionSecrets() (*ProductionSecrets, error) {
	vaultAddr := os.Getenv("VAULT_ADDR")
	vaultToken := os.Getenv("VAULT_TOKEN")
	secretsPath := os.Getenv("VAULT_SECRETS_PATH")

	if vaultAddr == "" || vaultToken == "" || secretsPath == "" {
		return nil, fmt.Errorf(" Vault configuration missing: VAULT_ADDR, VAULT_TOKEN, and VAULT_SECRETS_PATH must be set")
	}

	secrets, err := fetchFromVault(vaultAddr, vaultToken, secretsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secrets from Vault: %w", err)
	}

	if secrets.JWTSecret == "" || secrets.AgentEnrollmentSecret == "" || secrets.DatabaseURL == "" {
		return nil, fmt.Errorf("incomplete secrets retrieved from Vault: one or more required secrets are missing")
	}

	return secrets, nil
}

func fetchFromVault(addr, token, path string) (*ProductionSecrets, error) {
	return nil, fmt.Errorf("Vault integration not implemented: please set DB_URL, JWT_SECRET, and AGENT_ENROLLMENT_SECRET environment variables directly")
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
