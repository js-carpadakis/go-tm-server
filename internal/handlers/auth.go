/**
 * @file auth.go
 * @package handlers
 * @brief HTTP handlers for user registration and login.
 *
 * Both endpoints accept a JSON body of {email, password} and respond with
 * a signed JWT on success.  Passwords are never stored in plaintext — only
 * a bcrypt hash is persisted.  Both login failure cases (wrong password,
 * unknown email) return the same 401 response to prevent email enumeration.
 */
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

/**
 * @brief Holds the dependencies required by the auth endpoints.
 *
 * Constructed once in server.New and shared across all requests; both
 * fields are safe for concurrent use.
 */
type AuthHandler struct {
	DB        *sql.DB ///< Database connection used to read/write user records.
	JWTSecret string  ///< HMAC secret used to sign tokens; must match the Auth middleware.
}

/**
 * @brief Request body shape accepted by both /auth/register and /auth/login.
 */
type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

/**
 * @brief Response body returned on successful register or login.
 */
type authResponse struct {
	Token string `json:"token"` ///< Signed JWT; include as "Authorization: Bearer <token>".
}

/**
 * @brief POST /auth/register — creates a new user account and returns a JWT.
 *
 * Validation rules:
 *   - email must be non-empty
 *   - password must be at least 8 characters
 *
 * On success the password is bcrypt-hashed before being stored, and the
 * new user's ID is returned from the INSERT via RETURNING so a second
 * SELECT is avoided.  The response is 201 Created with a signed token.
 *
 * Returns 409 Conflict when the email is already registered.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request.
 */
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email required and password must be at least 8 characters")
		return
	}

	// bcrypt.DefaultCost (~10 rounds) balances security and latency.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var userID int64
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id`,
		req.Email, string(hash),
	).Scan(&userID)
	if err != nil {
		// UNIQUE constraint on email is the only expected failure path here.
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	token, err := h.signToken(userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{Token: token})
}

/**
 * @brief POST /auth/login — verifies credentials and returns a JWT.
 *
 * Looks up the user by email, then uses bcrypt.CompareHashAndPassword to
 * verify the submitted password against the stored hash.  Both "unknown
 * email" and "wrong password" return 401 with the same message to prevent
 * an attacker from discovering which emails are registered.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request.
 */
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	var userID int64
	var hash string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash FROM users WHERE email = ?`, req.Email,
	).Scan(&userID, &hash)
	if err != nil {
		// sql.ErrNoRows means unknown email — same response as wrong password.
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.signToken(userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: token})
}

/**
 * @brief Creates and signs a JWT for the given user.
 *
 * Claims included:
 *   - user_id: the user's database ID (used by the Auth middleware)
 *   - email:   the user's email address
 *   - exp:     expiry timestamp 24 hours from now
 *
 * Signed with HMAC-SHA256 using h.JWTSecret.
 *
 * @param userID  The user's primary key from the database.
 * @param email   The user's email address.
 * @return        The compact-serialised signed token string, or an error.
 */
func (h *AuthHandler) signToken(userID int64, email string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.JWTSecret))
}
