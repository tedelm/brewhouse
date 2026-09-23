package middleware

import (
	"context"
	"net/http"
	"strings"

	"brewhouse/internal/auth"
)

type claimsContextKey struct{}

// Auth returns middleware that requires a valid Bearer JWT on the request.
func Auth(tokens *auth.TokenIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
				return
			}
			const prefix = "Bearer "
			if !strings.HasPrefix(header, prefix) {
				http.Error(w, `{"error":"invalid authorization"}`, http.StatusUnauthorized)
				return
			}
			claims, err := tokens.Parse(strings.TrimPrefix(header, prefix))
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsContextKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext returns JWT claims stored by Auth middleware.
func ClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(*auth.Claims)
	return claims, ok
}
