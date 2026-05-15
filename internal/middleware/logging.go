/**
 * @file logging.go
 * @package middleware
 * @brief HTTP request logging middleware.
 *
 * Wraps every response writer so the final status code can be captured
 * after the handler returns, then logs method, path, status, and elapsed
 * time in a single line.
 */
package middleware

import (
	"log"
	"net/http"
	"time"
)

/**
 * @brief http.ResponseWriter wrapper that records the written status code.
 *
 * The stdlib ResponseWriter does not expose the status code after
 * WriteHeader has been called, so this thin wrapper captures it.
 * status defaults to 200 so handlers that never call WriteHeader
 * (implicit 200) are logged correctly.
 */
type responseWriter struct {
	http.ResponseWriter        ///< Embedded so all other methods are promoted unchanged.
	status             int     ///< HTTP status code written by the handler.
}

/**
 * @brief Intercepts WriteHeader to record the status code before forwarding.
 *
 * @param code  The HTTP status code the handler is writing.
 */
func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

/**
 * @brief Middleware that logs each request after the handler completes.
 *
 * Wraps the response writer, delegates to next, then logs:
 *   METHOD /path STATUS elapsed
 *
 * Placed outermost in the middleware chain so it captures the total
 * request duration including auth checks and handler work.
 *
 * @param next  The next handler in the chain.
 * @return      An http.Handler that logs and then calls next.
 */
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}
