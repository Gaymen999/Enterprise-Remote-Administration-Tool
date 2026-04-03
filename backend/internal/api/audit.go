package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/enterprise-rat/backend/internal/api/middleware"
	"github.com/enterprise-rat/backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditLogEntry struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	Username     string                 `json:"username"`
	Action       string                 `json:"action"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	IPAddress    string                 `json:"ip_address"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

type AuditLogFilter struct {
	UserID       string
	Action       string
	ResourceType string
	StartDate    *time.Time
	EndDate      *time.Time
	Limit        int
	Offset       int
}

func auditMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapper, r)

			// Extract claims and other info before starting the goroutine
			// to avoid race conditions and context cancellation issues.
			username := "anonymous"
			userID := ""
			claimsVal := r.Context().Value(middleware.ClaimsContextKey)
			if claimsVal != nil {
				if claims, ok := claimsVal.(*auth.Claims); ok {
					username = claims.Username
					userID = claims.UserID
				}
			}

			method := r.Method
			path := r.URL.Path
			query := r.URL.RawQuery
			userAgent := r.UserAgent()
			clientIP := getClientIP(r)
			resourceID := chi.URLParam(r, "id")

			go func() {
				if shouldSkipAudit(path) {
					return
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				action := determineAction(method, path)
				resourceType := determineResourceType(path)

				entry := AuditLogEntry{
					UserID:       userID,
					Username:     username,
					Action:       action,
					ResourceType: resourceType,
					ResourceID:   resourceID,
					Details: map[string]interface{}{
						"method":      method,
						"path":        path,
						"status_code": wrapper.statusCode,
						"duration_ms": time.Since(start).Milliseconds(),
						"query":       query,
					},
					IPAddress: clientIP,
					UserAgent: userAgent,
					CreatedAt: time.Now(),
				}

				if err := saveAuditLog(ctx, pool, entry); err != nil {
					log.Printf("[AUDIT] Failed to save audit log: %v", err)
				}
			}()
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func shouldSkipAudit(path string) bool {
	skipPaths := map[string]bool{
		"/health":              true,
		"/api/v1/auth/refresh": true,
		"/metrics":             true,
	}
	return skipPaths[path]
}

func determineAction(method, path string) string {
	actionMap := map[string]string{
		"GET":    "read",
		"POST":   "create",
		"PUT":    "update",
		"PATCH":  "update",
		"DELETE": "delete",
	}

	if action, ok := actionMap[method]; ok {
		return action
	}
	return method
}

func determineResourceType(path string) string {
	path = strings.TrimPrefix(path, "/api/v1/")

	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		resource := strings.TrimSuffix(parts[0], "s")
		if resource == "agent" || resource == "command" || resource == "file" || resource == "audit" {
			return resource
		}
		return resource
	}
	return "unknown"
}

func saveAuditLog(ctx context.Context, pool *pgxpool.Pool, entry AuditLogEntry) error {
	detailsJSON, err := json.Marshal(entry.Details)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO audit_logs (id, user_id, action, resource_type, resource_id, details, ip_address, created_at)
		VALUES (gen_random_uuid()::uuid, $1, $2, $3, $4, $5, $6::inet, NOW())
	`

	var userID interface{}
	if entry.UserID != "" {
		userID = entry.UserID
	}

	var resourceID interface{}
	if entry.ResourceID != "" {
		resourceID = entry.ResourceID
	}

	var ipAddress interface{}
	if entry.IPAddress != "" {
		ipAddress = entry.IPAddress
	}

	_, err = pool.Exec(ctx, query, userID, entry.Action, entry.ResourceType, resourceID, detailsJSON, ipAddress)
	return err
}

func GetAuditLogs(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		filter := AuditLogFilter{
			Limit:  100,
			Offset: 0,
		}

		if limit := r.URL.Query().Get("limit"); limit != "" {
			if parsed, err := strconv.Atoi(limit); err == nil && parsed > 0 {
				filter.Limit = parsed
			}
		}

		if offset := r.URL.Query().Get("offset"); offset != "" {
			if parsed, err := strconv.Atoi(offset); err == nil && parsed >= 0 {
				filter.Offset = parsed
			}
		}

		filter.Action = r.URL.Query().Get("action")
		filter.ResourceType = r.URL.Query().Get("resource_type")

		logs, err := fetchAuditLogs(r.Context(), pool, filter)
		if err != nil {
			log.Printf("[AUDIT] Failed to fetch audit logs: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch audit logs"})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logs":   logs,
			"count":  len(logs),
			"limit":  filter.Limit,
			"offset": filter.Offset,
		})
	}
}

func fetchAuditLogs(ctx context.Context, pool *pgxpool.Pool, filter AuditLogFilter) ([]AuditLogEntry, error) {
	query := `
		SELECT 
			id, 
			COALESCE(user_id::text, '') as user_id,
			COALESCE(action, '') as action,
			COALESCE(resource_type, '') as resource_type,
			COALESCE(resource_id::text, '') as resource_id,
			COALESCE(details::text, '{}') as details,
			COALESCE(ip_address::text, '') as ip_address,
			created_at
		FROM audit_logs
		WHERE 1=1
	`

	args := []interface{}{}
	argNum := 1

	if filter.UserID != "" {
		query += " AND user_id = $" + strconv.Itoa(argNum)
		args = append(args, filter.UserID)
		argNum++
	}

	if filter.Action != "" {
		query += " AND action = $" + strconv.Itoa(argNum)
		args = append(args, filter.Action)
		argNum++
	}

	if filter.ResourceType != "" {
		query += " AND resource_type = $" + strconv.Itoa(argNum)
		args = append(args, filter.ResourceType)
		argNum++
	}

	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(argNum)
	args = append(args, filter.Limit)
	argNum++

	query += " OFFSET $" + strconv.Itoa(argNum)
	args = append(args, filter.Offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLogEntry
	for rows.Next() {
		var entry AuditLogEntry
		var detailsJSON string
		var ipAddress string

		err := rows.Scan(
			&entry.ID,
			&entry.UserID,
			&entry.Action,
			&entry.ResourceType,
			&entry.ResourceID,
			&detailsJSON,
			&ipAddress,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(detailsJSON), &entry.Details); err != nil {
			log.Printf("[AUDIT] Failed to parse details JSON for log %s: %v", entry.ID, err)
			entry.Details = map[string]interface{}{}
		}
		entry.IPAddress = ipAddress

		logs = append(logs, entry)
	}

	return logs, nil
}

func LogAction(ctx context.Context, pool *pgxpool.Pool, userID, action, resourceType, resourceID string, details map[string]interface{}) error {
	entry := AuditLogEntry{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		CreatedAt:    time.Now(),
	}
	return saveAuditLog(ctx, pool, entry)
}
