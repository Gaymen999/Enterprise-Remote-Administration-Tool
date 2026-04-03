package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/enterprise-rat/backend/internal/auth"
)

type contextKey string

const ClaimsContextKey contextKey = "claims"

func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// Try getting from cookie
				if cookie, err := r.Cookie("access_token"); err == nil {
					authHeader = "Bearer " + cookie.Value
				}
			}

			if authHeader == "" {
				http.Error(w, `{"error": "missing authorization"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error": "invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateToken(parts[1], jwtSecret)
			if err != nil {
				log.Printf("[AUTH] Invalid token: %v", err)
				http.Error(w, `{"error": "invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

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

func GetClaims(ctx context.Context) *auth.Claims {
	if claims, ok := ctx.Value(ClaimsContextKey).(*auth.Claims); ok {
		return claims
	}
	return nil
}

func GetUserID(ctx context.Context) string {
	if claims := GetClaims(ctx); claims != nil {
		return claims.UserID
	}
	return ""
}
