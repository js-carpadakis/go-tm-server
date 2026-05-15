/**
 * @file server.go
 * @package server
 * @brief HTTP router and middleware wiring.
 *
 * New() is the single composition root for the application: it instantiates
 * all handlers, applies middleware, and registers every route on a stdlib
 * ServeMux.  The returned http.Handler is ready to pass directly to
 * http.ListenAndServe or httptest.NewServer.
 *
 * Route dispatch for /tasks/{id}/... is done manually because Go 1.22's
 * enhanced mux patterns only match exact suffixes; prefix-stripping for
 * sub-resources (tags) is handled by string inspection inside the handler.
 */
package server

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/jcarp/goserver/internal/handlers"
	"github.com/jcarp/goserver/internal/middleware"
)

/**
 * @brief Constructs and returns the fully wired HTTP handler for the application.
 *
 * Route table:
 *
 *   POST   /auth/register          — public, no JWT required
 *   POST   /auth/login             — public, no JWT required
 *   GET    /tasks                  — JWT required
 *   POST   /tasks                  — JWT required
 *   GET    /tasks/{id}             — JWT required
 *   PUT    /tasks/{id}             — JWT required
 *   DELETE /tasks/{id}             — JWT required
 *   POST   /tasks/{id}/tags        — JWT required
 *   DELETE /tasks/{id}/tags/{tag}  — JWT required
 *
 * Middleware chain (outermost to innermost):
 *   Logging → Auth (protected routes only) → handler
 *
 * @param db         An open, schema-initialised database connection.
 * @param jwtSecret  The HMAC secret used to sign and verify JWTs.
 *                   Must be the same value across all instances.
 * @return           The composed http.Handler to pass to the HTTP server.
 */
func New(db *sql.DB, jwtSecret string) http.Handler {
	mux := http.NewServeMux()

	auth := &handlers.AuthHandler{DB: db, JWTSecret: jwtSecret}
	tasks := &handlers.TaskHandler{DB: db}

	authMiddleware := middleware.Auth(jwtSecret)

	// --- Public routes (no JWT) ---
	mux.HandleFunc("/auth/register", auth.Register)
	mux.HandleFunc("/auth/login", auth.Login)

	// --- /tasks  (collection: list + create) ---
	mux.Handle("/tasks", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasks.List(w, r)
		case http.MethodPost:
			tasks.Create(w, r)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})))

	// --- /tasks/{id} and /tasks/{id}/tags[/{tag}] ---
	// The trailing slash causes ServeMux to route all /tasks/... here.
	// Sub-resource dispatch is done by inspecting the path string.
	mux.Handle("/tasks/", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// /tasks/{id}/tags/{tag}  — must be checked before /tags suffix
		if strings.Contains(path, "/tags/") {
			if r.Method == http.MethodDelete {
				tasks.RemoveTag(w, r)
			} else {
				http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		// /tasks/{id}/tags
		if strings.HasSuffix(path, "/tags") {
			if r.Method == http.MethodPost {
				tasks.AddTag(w, r)
			} else {
				http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		// /tasks/{id}
		switch r.Method {
		case http.MethodGet:
			tasks.Get(w, r)
		case http.MethodPut:
			tasks.Update(w, r)
		case http.MethodDelete:
			tasks.Delete(w, r)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})))

	// Logging wraps the entire mux so every request is logged regardless of route.
	return middleware.Logging(mux)
}
