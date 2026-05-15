/**
 * @file auth.go
 * @package middleware
 * @brief JWT authentication middleware and context helpers.
 *
 * Auth() returns a middleware that validates the Bearer token on every
 * protected request and injects the authenticated user's ID into the
 * request context so downstream handlers can read it without re-parsing
 * the token.
 */
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

/// contextKey is an unexported type used for context keys in this package.
/// Using a named type prevents collisions with keys from other packages.
type contextKey string

/// UserIDKey is the context key under which the authenticated user's ID is stored.
const UserIDKey contextKey = "userID"

/**
 * @brief Returns a middleware that enforces JWT authentication.
 *
 * The middleware reads the Authorization header, expects the value to begin
 * with "Bearer ", parses and validates the JWT using HMAC-SHA256 with the
 * provided secret, then extracts the "user_id" claim and stores it in the
 * request context under UserIDKey.
 *
 * Requests that are missing the header, carry an expired token, or use a
 * wrong signing method are rejected with 401 before reaching the handler.
 *
 * @param secret  The HMAC secret used to verify token signatures.
 *                Must match the secret used when signing tokens in AuthHandler.
 * @return        A middleware constructor: func(http.Handler) http.Handler.
 */
func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")

			// Parse the token, rejecting anything that isn't signed with HMAC
			// to prevent the "alg:none" and RSA/HMAC confusion attacks.
			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
				return
			}

			// JWT numbers are decoded as float64; cast to int64 for the context.
			userID, ok := claims["user_id"].(float64)
			if !ok {
				http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, int64(userID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

/**
 * @brief Retrieves the authenticated user's ID from a request context.
 *
 * Returns 0 if the value is absent, which only happens when the handler is
 * called without the Auth middleware — should not occur in production routing.
 *
 * @param ctx  The request context populated by the Auth middleware.
 * @return     The user ID stored by Auth, or 0 if not present.
 */
func UserIDFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(UserIDKey).(int64)
	return id
}
