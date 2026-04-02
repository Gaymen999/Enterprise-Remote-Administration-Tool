package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/enterprise-rat/backend/internal/auth"
	"github.com/enterprise-rat/backend/pkg/db"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

var allowedOrigins = parseWsOrigins()

type rateLimitEntry struct {
	count     int
	resetTime time.Time
}

var (
	rateLimitMap    = make(map[string]*rateLimitEntry)
	rateLimitMu     sync.Mutex
	rateLimitMax    = 100
	rateLimitWindow = 1 * time.Second
)

func parseWsOrigins() map[string]bool {
	envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if envOrigins == "" {
		return map[string]bool{}
	}

	origins := strings.Split(envOrigins, ",")
	result := make(map[string]bool)
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o != "" {
			result[o] = true
		}
	}
	log.Printf("[WS] Loaded %d allowed origins for WebSocket", len(result))
	return result
}

func isProduction() bool {
	return os.Getenv("ENV") == "production" || os.Getenv("ENV") == "prod"
}

func checkRateLimit(ip string) bool {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()

	now := time.Now()
	entry, exists := rateLimitMap[ip]

	if !exists || now.After(entry.resetTime) {
		rateLimitMap[ip] = &rateLimitEntry{
			count:     1,
			resetTime: now.Add(rateLimitWindow),
		}
		return true
	}

	if entry.count >= rateLimitMax {
		log.Printf("[WS] Rate limit exceeded for IP: %s", ip)
		return false
	}

	entry.count++
	return true
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")

		if origin == "" {
			if isProduction() {
				log.Printf("[WS] SECURITY: Rejected request with empty origin in production")
				return false
			}
			return true
		}

		allowed := allowedOrigins[origin]
		if !allowed {
			log.Printf("[WS] SECURITY: Blocked cross-origin WebSocket from: %s", origin)
		}
		return allowed
	},
}

type Handler struct {
	hub       *Hub
	jwtSecret string
	dbPool    *pgxpool.Pool
}

func NewHandler(hub *Hub, jwtSecret string, pool *pgxpool.Pool) *Handler {
	return &Handler{
		hub:       hub,
		jwtSecret: jwtSecret,
		dbPool:    pool,
	}
}

func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if !checkRateLimit(clientIP) {
		http.Error(w, `{"error": "rate limit exceeded"}`, http.StatusTooManyRequests)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade failed: %v", err)
		return
	}

	clientType, clientID, err := h.authenticate(r)
	if err != nil {
		log.Printf("[WS] Authentication failed: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error": "authentication failed"}`))
		conn.Close()
		return
	}

	client := NewClient(clientID, clientType, conn, h.hub)
	h.hub.Register(client)
	client.StartPump()

	go client.writePump()
	go h.readPump(client)
}

func (h *Handler) authenticate(r *http.Request) (ClientType, string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", "", errors.New("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid authorization header format")
	}

	claims, err := auth.ValidateToken(parts[1], h.jwtSecret)
	if err != nil {
		return "", "", err
	}

	if claims.Role == "agent" {
		return ClientTypeAgent, claims.UserID, nil
	}
	return ClientTypeAdmin, claims.UserID, nil
}

type WSSMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

func (h *Handler) readPump(client *Client) {
	defer func() {
		h.hub.Unregister(client)
		client.Conn.Close()
	}()

	const (
		pongWait       = 60 * time.Second
		pingInterval   = 30 * time.Second
		maxMessageSize = 4 * 1024 * 1024
	)
	client.Conn.SetReadLimit(maxMessageSize)
	client.Conn.SetReadDeadline(time.Now().Add(pongWait))
	client.Conn.SetPongHandler(func(string) error {
		log.Printf("[WS] Pong received from %s %s", client.Type, client.ID)
		client.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	log.Printf("[WS] readPump started for %s %s", client.Type, client.ID)

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] Read error for %s %s: %v", client.Type, client.ID, err)
			} else {
				log.Printf("[WS] Connection closed for %s %s: %v", client.Type, client.ID, err)
			}
			break
		}

		var msg WSSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[WS] Invalid JSON from %s %s: %v", client.Type, client.ID, err)
			continue
		}

		h.handleMessage(client, msg)
	}

	log.Printf("[WS] readPump ended for %s %s", client.Type, client.ID)
}

func (h *Handler) handleMessage(client *Client, msg WSSMessage) {
	switch msg.Type {
	case "agent_register":
		h.handleAgentRegister(client, msg.Payload)
	case "heartbeat":
		h.handleHeartbeat(client, msg.Payload)
	case "command_result":
		h.handleCommandResult(client, msg.Payload)
	case "pty":
		h.handlePtyMessage(client, msg)
	case "pty_output":
		h.handlePtyOutput(client, msg.Payload)
	default:
		log.Printf("[WS] Unknown message type from %s %s: %s", client.Type, client.ID, msg.Type)
	}
}

func (h *Handler) handleAgentRegister(client *Client, payload map[string]interface{}) {
	if payload == nil {
		return
	}

	agentID := getString(payload, "agent_id")
	hostname := sanitizeHostname(getString(payload, "hostname"))
	osFamily := sanitizeOSInfo(getString(payload, "os_family"))
	osVersion := sanitizeOSInfo(getString(payload, "os_version"))

	if agentID == "" {
		agentID = client.ID
	}

	if !isValidUUID(agentID) && agentID != client.ID {
		log.Printf("[WS] Invalid agent ID format: %s, using client ID", agentID)
		agentID = client.ID
	}

	if hostname == "" {
		hostname = "unknown"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := db.AgentInfo{
		AgentID:   agentID,
		Hostname:  hostname,
		OSFamily:  osFamily,
		OSVersion: osVersion,
	}

	if err := db.UpsertAgent(ctx, h.dbPool, info); err != nil {
		log.Printf("[WS] Failed to upsert agent %s: %v", agentID, err)
		return
	}

	log.Printf("[WS] Agent registered in DB: %s (%s)", hostname, agentID)
}

func sanitizeHostname(hostname string) string {
	hostname = strings.TrimSpace(hostname)
	hostname = strings.ReplaceAll(hostname, "<", "")
	hostname = strings.ReplaceAll(hostname, ">", "")
	hostname = strings.ReplaceAll(hostname, ";", "")
	hostname = strings.ReplaceAll(hostname, "&", "")
	hostname = strings.ReplaceAll(hostname, "\"", "")
	hostname = strings.ReplaceAll(hostname, "'", "")
	if len(hostname) > 255 {
		hostname = hostname[:255]
	}
	return hostname
}

func sanitizeOSInfo(info string) string {
	info = strings.TrimSpace(info)
	info = strings.ReplaceAll(info, "<", "")
	info = strings.ReplaceAll(info, ">", "")
	if len(info) > 100 {
		info = info[:100]
	}
	return info
}

func isValidUUID(u string) bool {
	uuidRegex := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	matched, _ := regexp.MatchString(uuidRegex, u)
	return matched
}

func (h *Handler) handleHeartbeat(client *Client, payload map[string]interface{}) {
	if client.Type != ClientTypeAgent {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db.UpdateAgentStatus(ctx, h.dbPool, client.ID, "online")
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (h *Handler) handleCommandResult(client *Client, payload map[string]interface{}) {
	if payload == nil {
		return
	}

	commandID := getString(payload, "command_id")
	stdout := getString(payload, "stdout")
	stderr := getString(payload, "stderr")
	exitCode := 0
	if code, ok := payload["exit_code"].(float64); ok {
		exitCode = int(code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.SaveCommandResult(ctx, h.dbPool, commandID, stdout, stderr, exitCode); err != nil {
		log.Printf("[WS] Failed to save command result for %s: %v", commandID, err)
		return
	}

	log.Printf("[WS] Command result saved: %s (exit: %d)", commandID, exitCode)
}
