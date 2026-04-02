package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/enterprise-rat/backend/internal/auth"
	"github.com/enterprise-rat/backend/internal/models"
	"github.com/enterprise-rat/backend/internal/ws"
	"github.com/enterprise-rat/backend/pkg/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type RouterDeps struct {
	JWTSecret string
	Hub       *ws.Hub
	DBPool    *pgxpool.Pool
}

var corsAllowedOrigins = parseCorsOrigins()

func parseCorsOrigins() []string {
	envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if envOrigins == "" {
		return []string{}
	}

	origins := strings.Split(envOrigins, ",")
	result := make([]string, 0, len(origins))
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o != "" {
			result = append(result, o)
		}
	}
	log.Printf("[CORS] Loaded %d allowed origins from environment", len(result))
	return result
}

func isOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range corsAllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	log.Printf("[CORS] Origin not allowed: %s", origin)
	return false
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func rateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.Allow(getClientIP(r)) {
				http.Error(w, `{"error": "rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)
	r.Use(secureHeaders)

	loginRateLimiter := NewRateLimiter(3, 1*time.Minute)

	r.Get("/health", healthHandler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(rateLimitMiddleware(loginRateLimiter))
			r.Post("/auth/login", loginHandler(deps.DBPool, deps.JWTSecret))
			r.Post("/auth/refresh", refreshTokenHandler(deps.DBPool, deps.JWTSecret))
			r.Post("/auth/logout", logoutHandler())
			r.Post("/auth/register", registerHandler)
		})

		r.Group(func(r chi.Router) {
			r.Use(authMiddleware(deps.JWTSecret))
			r.Use(auditMiddleware(deps.DBPool))

			r.Get("/agents", listAgentsHandler(deps.DBPool))
			r.Get("/agents/{id}", getAgentHandler(deps.DBPool))
			r.Post("/commands", createCommandHandler(deps.DBPool, deps.Hub))
			r.Get("/commands/{id}", getCommandHandler(deps.DBPool))
			r.Get("/commands/{id}/result", getCommandResultHandler(deps.DBPool))

			r.Post("/files/manage", fileManagerHandler(deps.Hub))
			r.Get("/files/download/{agentId}", fileDownloadHandler(deps.Hub))
			r.Post("/files/upload/{agentId}", fileUploadHandler(deps.Hub))

			r.Get("/audit", GetAuditLogs(deps.DBPool))
		})

		r.Get("/ws", wsHandler(deps.Hub, deps.JWTSecret, deps.DBPool))
	})

	return r
}

func wsHandler(hub *ws.Hub, jwtSecret string, pool *pgxpool.Pool) http.HandlerFunc {
	handler := ws.NewHandler(hub, jwtSecret, pool)
	return handler.HandleWS
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "ok"}`))
}

func authMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				cookie, err := r.Cookie("access_token")
				if err != nil {
					http.Error(w, `{"error": "missing authorization"}`, http.StatusUnauthorized)
					return
				}
				authHeader = "Bearer " + cookie.Value
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error": "invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateToken(parts[1], secret)
			if err != nil {
				log.Printf("[AUTH] Invalid token: %v", err)
				http.Error(w, `{"error": "invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			log.Printf("[AUTH] Protected route accessed by %s (%s): %s %s", claims.Username, claims.Role, r.Method, r.URL.Path)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type contextKey string

const ClaimsContextKey contextKey = "claims"

func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claimsVal := r.Context().Value(ClaimsContextKey)
			if claimsVal == nil {
				http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, ok := claimsVal.(*auth.Claims)
			if !ok {
				http.Error(w, `{"error": "invalid claims"}`, http.StatusInternalServerError)
				return
			}

			roleAllowed := false
			for _, role := range allowedRoles {
				if claims.Role == role {
					roleAllowed = true
					break
				}
			}

			if !roleAllowed {
				log.Printf("[AUTH] Access denied for user %s with role %s", claims.Username, claims.Role)
				http.Error(w, `{"error": "insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "self-registration is disabled"})
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	User        *UserResponse `json:"user"`
	AccessToken string        `json:"access_token"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func loginHandler(pool *pgxpool.Pool, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		if req.Username == "" || req.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "username and password required"})
			return
		}

		sanitizedUsername := sanitizeInput(req.Username)
		userID, passwordHash, role, err := db.GetUserByUsernameAndRole(r.Context(), pool, sanitizedUsername)
		if err != nil {
			log.Printf("[AUTH] Failed login attempt for user: %s", sanitizedUsername)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
			log.Printf("[AUTH] Invalid password for user: %s", sanitizedUsername)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
			return
		}

		tokenPair, err := auth.GenerateTokenPair(userID, sanitizedUsername, role, jwtSecret, 24, 168)
		if err != nil {
			log.Printf("[AUTH] Failed to generate token for user %s: %v", sanitizedUsername, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate token"})
			return
		}

		log.Printf("[AUTH] User logged in successfully: %s", sanitizedUsername)

		setTokenCookie(w, "access_token", tokenPair.AccessToken, 900)
		setTokenCookie(w, "refresh_token", tokenPair.RefreshToken, 86400)

		response := LoginResponse{
			User: &UserResponse{
				ID:       userID,
				Username: sanitizedUsername,
				Role:     role,
			},
			AccessToken: tokenPair.AccessToken,
		}

		json.NewEncoder(w).Encode(response)
	}
}

type RateLimiter struct {
	requests    map[string][]time.Time
	mu          sync.RWMutex
	limit       int
	window      time.Duration
	cleanupDone chan struct{}
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests:    make(map[string][]time.Time),
		limit:       limit,
		window:      window,
		cleanupDone: make(chan struct{}),
	}
	go rl.cleanupOldEntries()
	return rl
}

func (rl *RateLimiter) cleanupOldEntries() {
	ticker := time.NewTicker(5 * time.Minute)
	defer func() {
		ticker.Stop()
		close(rl.cleanupDone)
	}()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			windowStart := now.Add(-rl.window)
			for ip, times := range rl.requests {
				var validTimes []time.Time
				for _, t := range times {
					if t.After(windowStart) {
						validTimes = append(validTimes, t)
					}
				}
				if len(validTimes) == 0 {
					delete(rl.requests, ip)
				} else {
					rl.requests[ip] = validTimes
				}
			}
			rl.mu.Unlock()
		case <-rl.cleanupDone:
			return
		}
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	if times, ok := rl.requests[ip]; ok {
		var validTimes []time.Time
		for _, t := range times {
			if t.After(windowStart) {
				validTimes = append(validTimes, t)
			}
		}
		rl.requests[ip] = validTimes

		if len(validTimes) >= rl.limit {
			return false
		}
	}

	rl.requests[ip] = append(rl.requests[ip], now)
	return true
}

func sanitizeInput(input string) string {
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "<", "&lt;")
	input = strings.ReplaceAll(input, ">", "&gt;")
	input = strings.ReplaceAll(input, "\"", "&quot;")
	input = strings.ReplaceAll(input, "'", "&#x27;")
	return input
}

func getClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	trustedProxies := []string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

	if xff != "" {
		parts := strings.Split(xff, ",")
		firstIP := strings.TrimSpace(parts[0])
		if isTrustedProxy(firstIP, trustedProxies) {
			return firstIP
		}
		log.Printf("[SECURITY] Rejected spoofed X-Forwarded-For: %s", firstIP)
	}

	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		if isTrustedProxy(xri, trustedProxies) {
			return xri
		}
		log.Printf("[SECURITY] Rejected spoofed X-Real-IP: %s", xri)
	}

	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}

func isTrustedProxy(ip string, trusted []string) bool {
	for _, trustedNet := range trusted {
		if strings.HasSuffix(trustedNet, "/8") {
			prefix := strings.TrimSuffix(trustedNet, "/8")
			if strings.HasPrefix(ip, prefix+".") {
				return true
			}
		} else if strings.HasSuffix(trustedNet, "/12") {
			prefix := strings.TrimSuffix(trustedNet, "/12")
			if strings.HasPrefix(ip, prefix+".") {
				return true
			}
		} else if strings.HasSuffix(trustedNet, "/16") {
			prefix := strings.TrimSuffix(trustedNet, "/16")
			if strings.HasPrefix(ip, prefix+".") {
				return true
			}
		} else if ip == trustedNet {
			return true
		}
	}
	return false
}

func setTokenCookie(w http.ResponseWriter, name, value string, maxAge int) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   maxAge,
	}
	http.SetCookie(w, cookie)
}

func refreshTokenHandler(pool *pgxpool.Pool, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		cookie, err := r.Cookie("refresh_token")
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "no refresh token"})
			return
		}

		claims, err := auth.ValidateToken(cookie.Value, jwtSecret)
		if err != nil {
			log.Printf("[AUTH] Invalid refresh token: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid refresh token"})
			return
		}

		tokenPair, err := auth.GenerateTokenPair(claims.UserID, claims.Username, claims.Role, jwtSecret, 24, 168)
		if err != nil {
			log.Printf("[AUTH] Failed to generate new token pair: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to refresh token"})
			return
		}

		setTokenCookie(w, "access_token", tokenPair.AccessToken, 900)
		setTokenCookie(w, "refresh_token", tokenPair.RefreshToken, 86400)

		log.Printf("[AUTH] Token refreshed for user: %s", claims.Username)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "tokens refreshed"})
	}
}

func logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		clearCookie := &http.Cookie{
			Name:     "access_token",
			Value:    "",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
			MaxAge:   -1,
		}
		http.SetCookie(w, clearCookie)

		refreshCookie := &http.Cookie{
			Name:     "refresh_token",
			Value:    "",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
			MaxAge:   -1,
		}
		http.SetCookie(w, refreshCookie)

		log.Printf("[AUTH] User logged out")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
	}
}

func listAgentsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		agents, err := db.GetAgents(ctx, pool)
		if err != nil {
			log.Printf("[ERROR] Failed to fetch agents: %v", err)
			w.Write([]byte(`{"agents": [], "error": "failed to fetch agents"}`))
			return
		}

		log.Printf("[INFO] Found %d agents in DB", len(agents))

		response := struct {
			Agents []models.Agent `json:"agents"`
		}{Agents: agents}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("[ERROR] Failed to encode agents: %v", err)
		}
	}
}

func getAgentHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		agentID := chi.URLParam(r, "id")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		agent, err := db.GetAgentByID(ctx, pool, agentID)
		if err != nil {
			if err == db.ErrAgentNotFound {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "agent not found"})
				return
			}
			log.Printf("[ERROR] Failed to fetch agent %s: %v", agentID, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch agent"})
			return
		}

		json.NewEncoder(w).Encode(agent)
	}
}

func createCommandHandler(pool *pgxpool.Pool, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var req struct {
			AgentID    string   `json:"agent_id"`
			Executable string   `json:"executable"`
			Args       []string `json:"args"`
			Timeout    int      `json:"timeout_seconds"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		if req.AgentID == "" || req.Executable == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "agent_id and executable are required"})
			return
		}

		if req.Timeout <= 0 {
			req.Timeout = 300
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		commandID, err := db.CreateCommand(ctx, pool, req.AgentID, req.Executable, req.Args, req.Timeout)
		if err != nil {
			log.Printf("[CMD] Failed to create command: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create command"})
			return
		}

		wsMsg := map[string]interface{}{
			"type": "command",
			"payload": map[string]interface{}{
				"command_id":      commandID,
				"executable":      req.Executable,
				"args":            req.Args,
				"timeout_seconds": req.Timeout,
			},
		}

		msgBytes, _ := json.Marshal(wsMsg)
		if !hub.SendToAgent(req.AgentID, msgBytes) {
			log.Printf("[CMD] Agent %s not connected, command queued", req.AgentID)
		} else {
			log.Printf("[CMD] Command %s sent to agent %s", commandID, req.AgentID)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"command_id": commandID,
			"status":     "sent",
		})
	}
}

func getCommandHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		commandID := chi.URLParam(r, "id")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		command, err := db.GetCommandByID(ctx, pool, commandID)
		if err != nil {
			if err == db.ErrCommandNotFound {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "command not found"})
				return
			}
			log.Printf("[ERROR] Failed to fetch command %s: %v", commandID, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch command"})
			return
		}

		json.NewEncoder(w).Encode(command)
	}
}

func getCommandResultHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		commandID := chi.URLParam(r, "id")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		result, err := db.GetCommandResult(ctx, pool, commandID)
		if err != nil {
			if err == db.ErrCommandNotFound {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "command not found"})
				return
			}
			log.Printf("[ERROR] Failed to fetch command result for %s: %v", commandID, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch command result"})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"command_id": result.CommandID,
			"stdout":     result.Stdout,
			"stderr":     result.Stderr,
			"exit_code":  result.ExitCode,
			"completed":  result.Completed,
		})
	}
}
