package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/enterprise-rat/agent/internal/client"
	"github.com/enterprise-rat/agent/internal/config"
)

func main() {
	log.Println("Starting Enterprise RAT Agent...")

	identity, err := config.LoadOrCreateIdentity()
	if err != nil {
		log.Fatalf("Failed to load identity: %v", err)
	}
	log.Printf("Agent ID: %s | Host: %s | OS: %s", identity.AgentID, identity.Hostname, identity.OSFamily)

	serverURL := os.Getenv("RAT_SERVER_URL")
	if serverURL == "" {
		log.Fatal("RAT_SERVER_URL environment variable is required")
	}

	token := os.Getenv("RAT_AGENT_TOKEN")
	enrollmentSecret := os.Getenv("AGENT_ENROLLMENT_SECRET")

	log.Printf("[CONFIG] Server URL: %s", serverURL)
	log.Printf("[CONFIG] Enrollment Secret: %s", maskSecret(enrollmentSecret))

	wssClient := client.NewWSSClient(serverURL, token, enrollmentSecret, identity)
	go wssClient.Run()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down agent...")
	wssClient.Close()
}

func maskSecret(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}
