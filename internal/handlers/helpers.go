/**
 * @file helpers.go
 * @package handlers
 * @brief Shared HTTP response helpers used by all handler types.
 *
 * Centralising JSON serialisation here ensures every response sets the
 * correct Content-Type header and uses the same encoder, and that error
 * payloads always follow the {"error": "..."} shape expected by clients.
 */
package handlers

import (
	"encoding/json"
	"net/http"
)

/**
 * @brief Writes a JSON-encoded value with the given HTTP status code.
 *
 * Sets Content-Type to application/json before writing the header so the
 * header is not locked by an early WriteHeader call.
 *
 * @param w       The response writer to write into.
 * @param status  The HTTP status code (e.g. http.StatusOK).
 * @param v       Any value that can be marshalled to JSON.
 */
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

/**
 * @brief Writes a JSON error payload with the given HTTP status code.
 *
 * The response body is always {"error": "<msg>"} so clients have a single
 * consistent shape to handle for all error conditions.
 *
 * @param w       The response writer to write into.
 * @param status  The HTTP status code (e.g. http.StatusBadRequest).
 * @param msg     The human-readable error message.
 */
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
