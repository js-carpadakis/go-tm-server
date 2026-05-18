/**
 * @file tasks.go
 * @package handlers
 * @brief HTTP handlers for task CRUD, filtering, and tag management.
 *
 * All handlers enforce ownership by including "AND user_id = ?" in every
 * query — a user can only ever read or modify their own tasks.
 * The userID is read from the request context, where it was placed by the
 * Auth middleware after validating the JWT.
 */
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jcarp/goserver/internal/middleware"
	"github.com/jcarp/goserver/internal/models"
)

/**
 * @brief Holds the database connection shared across all task endpoints.
 */
type TaskHandler struct {
	DB *sql.DB ///< Database connection; safe for concurrent use.
}

/**
 * @brief JSON request body shape accepted by Create and Update.
 *
 * All fields are optional for Update (partial patch semantics).
 * A nil DueDate pointer means "do not change the existing due date".
 */
type taskRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	DueDate     *time.Time `json:"due_date"`
	Tags        []string   `json:"tags"`
}

/**
 * @brief GET /tasks — lists tasks owned by the authenticated user.
 *
 * Supports optional query parameters:
 *   - status=<value>   filter to tasks with exactly this status
 *   - tag=<name>       filter to tasks that have this tag
 *   - overdue=true     filter to tasks whose due_date is in the past and status != done
 *
 * Filters are additive (AND).  Results are ordered newest-first.
 * Always returns a JSON array (empty array when there are no results).
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request.
 */
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	status := r.URL.Query().Get("status")
	tag := r.URL.Query().Get("tag")
	overdue := r.URL.Query().Get("overdue") == "true"

	// Base query joins tags so we can filter by tag name in the same pass.
	// DISTINCT prevents duplicate rows when a task has multiple tags.
	query := `
		SELECT DISTINCT t.id, t.user_id, t.title, t.description, t.status, t.due_date, t.created_at, t.updated_at
		FROM tasks t
		LEFT JOIN task_tags tt ON tt.task_id = t.id
		LEFT JOIN tags tg ON tg.id = tt.tag_id
		WHERE t.user_id = ?`
	args := []any{userID}

	if status != "" {
		query += " AND t.status = ?"
		args = append(args, status)
	}
	if tag != "" {
		query += " AND tg.name = ?"
		args = append(args, tag)
	}
	if overdue {
		query += " AND t.due_date IS NOT NULL AND t.due_date < datetime('now') AND t.status != 'done'"
	}
	query += " ORDER BY t.created_at DESC"

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	tasks := []models.Task{} // initialised to [] so JSON output is [] not null
	for rows.Next() {
		var task models.Task
		var dueDateStr sql.NullString
		if err := rows.Scan(&task.ID, &task.UserID, &task.Title, &task.Description,
			&task.Status, &dueDateStr, &task.CreatedAt, &task.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if dueDateStr.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", dueDateStr.String)
			task.DueDate = &t
		}
		task.Tags = h.fetchTags(r, task.ID)
		tasks = append(tasks, task)
	}
	writeJSON(w, http.StatusOK, tasks)
}

/**
 * @brief POST /tasks — creates a new task for the authenticated user.
 *
 * Title is the only required field; status defaults to "todo" if omitted.
 * Tags are upserted and linked in a single syncTags call.
 * Responds 201 Created with the full task object including its new ID and tags.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request.
 */
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	status := models.Status(req.Status)
	if req.Status == "" {
		status = models.StatusTodo
	} else if !status.Valid() {
		writeError(w, http.StatusBadRequest, "status must be todo, in-progress, or done")
		return
	}

	var taskID int64
	err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO tasks (user_id, title, description, status, due_date) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		userID, req.Title, req.Description, string(status), timePtr(req.DueDate),
	).Scan(&taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.syncTags(r, taskID, req.Tags); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	task := h.Task(r, taskID, userID)
	if task == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

/**
 * @brief GET /tasks/{id} — returns a single task with its tags.
 *
 * Enforces ownership: returns 404 (not 403) when the task exists but belongs
 * to a different user, to avoid leaking whether the ID is valid.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request; task ID is parsed from the URL path.
 */
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	taskID, err := idFromPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	task := h.Task(r, taskID, userID)
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

/**
 * @brief PUT /tasks/{id} — partially updates a task.
 *
 * Only fields present with non-zero values in the request body are applied;
 * omitted fields keep their current database value.  This is achieved in a
 * single UPDATE using COALESCE(NULLIF(?, ''), column) so no read-then-write
 * is needed for scalar fields.
 *
 * If Tags is present in the body (even as an empty array) the tag list is
 * fully replaced via syncTags.  If Tags is absent (nil) tags are unchanged.
 *
 * Responds 200 OK with the updated task on success, 404 when the task does
 * not exist or belongs to another user.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request; task ID is parsed from the URL path.
 */
func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	taskID, err := idFromPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Status != "" && !models.Status(req.Status).Valid() {
		writeError(w, http.StatusBadRequest, "status must be todo, in-progress, or done")
		return
	}

	// COALESCE(NULLIF(?, ''), col) keeps the old value when the new value is empty.
	// due_date uses a sentinel int (boolInt) because NULL is a valid target value.
	res, err := h.DB.ExecContext(r.Context(), `
		UPDATE tasks SET
			title       = COALESCE(NULLIF(?, ''), title),
			description = CASE WHEN ? != '' THEN ? ELSE description END,
			status      = COALESCE(NULLIF(?, ''), status),
			due_date    = CASE WHEN ? = 1 THEN ? ELSE due_date END,
			updated_at  = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?`,
		req.Title,
		req.Description, req.Description,
		req.Status,
		boolInt(req.DueDate != nil), timePtr(req.DueDate),
		taskID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Only replace tags when the caller explicitly included the field.
	if req.Tags != nil {
		if err := h.syncTags(r, taskID, req.Tags); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	task := h.Task(r, taskID, userID)
	writeJSON(w, http.StatusOK, task)
}

/**
 * @brief DELETE /tasks/{id} — permanently removes a task.
 *
 * Cascading deletes in the schema automatically remove associated task_tags
 * rows.  Returns 204 No Content on success, 404 when the task is not found
 * or belongs to another user.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request; task ID is parsed from the URL path.
 */
func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	taskID, err := idFromPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	res, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM tasks WHERE id = ? AND user_id = ?`, taskID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

/**
 * @brief POST /tasks/{id}/tags — appends tags to a task.
 *
 * Merges the submitted tags with the existing ones (no duplicates) and
 * writes the combined set back via syncTags.  Responds 200 OK with the
 * updated task.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request; expects body {"tags": ["a","b"]}.
 */
func (h *TaskHandler) AddTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	taskID, err := idFromPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Tags) == 0 {
		writeError(w, http.StatusBadRequest, "tags array required")
		return
	}

	if h.Task(r, taskID, userID) == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	existing := h.fetchTags(r, taskID)
	merged := unique(append(existing, body.Tags...))
	if err := h.syncTags(r, taskID, merged); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, h.Task(r, taskID, userID))
}

/**
 * @brief DELETE /tasks/{id}/tags/{tag} — removes a single tag from a task.
 *
 * Fetches the current tag list, filters out the named tag, and writes the
 * remainder back via syncTags.  If the tag was not present the call is still
 * successful (idempotent).  Responds 200 OK with the updated task.
 *
 * @param w  The HTTP response writer.
 * @param r  The incoming HTTP request; tag name is the last path segment.
 */
func (h *TaskHandler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	taskID, err := idFromPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	// Path shape: /tasks/{id}/tags/{tag}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "tag name required")
		return
	}
	tagName := parts[3]

	if h.Task(r, taskID, userID) == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	existing := h.fetchTags(r, taskID)
	updated := filter(existing, func(t string) bool { return t != tagName })
	if err := h.syncTags(r, taskID, updated); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, h.Task(r, taskID, userID))
}

// --- internal helpers ---

/**
 * @brief Fetches a single task with its tags, scoped to a specific user.
 *
 * Used internally after create/update to return the full object, and by
 * AddTag/RemoveTag to verify ownership before modifying tags.
 * Returns nil when the task does not exist or belongs to a different user.
 *
 * @param r       The current request (for context and cancellation).
 * @param taskID  Primary key of the task to fetch.
 * @param userID  Must match the task's user_id column.
 * @return        Pointer to the populated Task, or nil if not found.
 */
func (h *TaskHandler) Task(r *http.Request, taskID, userID int64) *models.Task {
	var task models.Task
	var dueDateStr sql.NullString
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT id, user_id, title, description, status, due_date, created_at, updated_at
		FROM tasks WHERE id = ? AND user_id = ?`, taskID, userID,
	).Scan(&task.ID, &task.UserID, &task.Title, &task.Description,
		&task.Status, &dueDateStr, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil
	}
	if dueDateStr.Valid {
		t, _ := time.Parse("2006-01-02T15:04:05Z", dueDateStr.String)
		task.DueDate = &t
	}
	task.Tags = h.fetchTags(r, taskID)
	return &task
}

/**
 * @brief Returns the alphabetically ordered tag names for a task.
 *
 * Performs a JOIN across task_tags and tags.  Returns nil (not an empty
 * slice) when there are no tags, which omits the field from JSON output
 * via the omitempty tag on Task.Tags.
 *
 * @param r       The current request (for context and cancellation).
 * @param taskID  Primary key of the task whose tags to retrieve.
 * @return        Slice of tag name strings, or nil if none.
 */
func (h *TaskHandler) fetchTags(r *http.Request, taskID int64) []string {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT tg.name FROM tags tg
		JOIN task_tags tt ON tt.tag_id = tg.id
		WHERE tt.task_id = ? ORDER BY tg.name`, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tags = append(tags, name)
	}
	return tags
}

/**
 * @brief Replaces the full tag set for a task.
 *
 * Strategy: delete all existing task_tags rows for the task, then re-insert.
 * Each tag name is upserted into the global tags table
 * (ON CONFLICT DO UPDATE) to retrieve its ID, then linked via task_tags.
 * Duplicates and blank strings in the input are silently ignored.
 *
 * @param r       The current request (for context and cancellation).
 * @param taskID  Primary key of the task to update.
 * @param tags    The desired complete tag list after the operation.
 * @return        Any database error encountered, or nil on success.
 */
func (h *TaskHandler) syncTags(r *http.Request, taskID int64, tags []string) error {
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM task_tags WHERE task_id = ?`, taskID); err != nil {
		return err
	}

	for _, name := range unique(tags) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		var tagID int64
		err := h.DB.QueryRowContext(r.Context(),
			`INSERT INTO tags (name) VALUES (?) ON CONFLICT(name) DO UPDATE SET name=name RETURNING id`, name,
		).Scan(&tagID)
		if err != nil {
			return err
		}

		if _, err := h.DB.ExecContext(r.Context(),
			`INSERT OR IGNORE INTO task_tags (task_id, tag_id) VALUES (?, ?)`, taskID, tagID); err != nil {
			return err
		}
	}
	return nil
}

/**
 * @brief Parses the task ID from the second path segment of a URL.
 *
 * Expects paths in the form /tasks/{id} or /tasks/{id}/....
 * Returns strconv.ErrSyntax when the segment is missing or non-numeric.
 *
 * @param path  The full URL path string (e.g. "/tasks/42/tags").
 * @return      The parsed task ID, or an error.
 */
func idFromPath(path string) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(parts[1], 10, 64)
}

/**
 * @brief Converts a *time.Time to a UTC string suitable for SQLite storage.
 *
 * Returns nil when t is nil, which causes the database column to be set to NULL.
 *
 * @param t  Pointer to the time value, or nil.
 * @return   Formatted string "2006-01-02T15:04:05Z", or nil.
 */
func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

/**
 * @brief Converts a bool to an int for use as a SQL sentinel value.
 *
 * Used in the UPDATE query to distinguish "caller wants to set due_date"
 * from "caller omitted due_date" — SQLite has no native boolean type.
 *
 * @param b  The boolean to convert.
 * @return   1 if b is true, 0 otherwise.
 */
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

/**
 * @brief Returns a deduplicated copy of ss preserving first-occurrence order.
 *
 * @param ss  Input string slice, may contain duplicates.
 * @return    New slice with duplicate entries removed.
 */
func unique(ss []string) []string {
	seen := map[string]bool{}
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

/**
 * @brief Filters a string slice, keeping only elements for which keep returns true.
 *
 * @param ss    Input string slice.
 * @param keep  Predicate function; returning true retains the element.
 * @return      Filtered slice (may be empty).
 */
func filter(ss []string, keep func(string) bool) []string {
	out := ss[:0]
	for _, s := range ss {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}
