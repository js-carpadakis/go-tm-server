/**
 * @file models.go
 * @package models
 * @brief Domain types shared across handlers, middleware, and tests.
 *
 * Defines the core data structures (User, Task) and the Status enum.
 * JSON struct tags control serialisation; the `json:"-"` tag on
 * PasswordHash ensures it is never leaked in API responses.
 */
package models

import "time"

/// Status is the lifecycle state of a Task.
/// Only the three named constants are considered valid.
type Status string

const (
	StatusTodo       Status = "todo"        ///< Task has not been started.
	StatusInProgress Status = "in-progress" ///< Task is actively being worked on.
	StatusDone       Status = "done"        ///< Task is complete.
)

/**
 * @brief Reports whether s is one of the three recognised Status values.
 *
 * Used during request validation to reject arbitrary strings before they
 * reach the database.
 *
 * @return true if s is "todo", "in-progress", or "done".
 */
func (s Status) Valid() bool {
	return s == StatusTodo || s == StatusInProgress || s == StatusDone
}

/**
 * @brief Represents an authenticated user account.
 *
 * PasswordHash is excluded from JSON output via the `json:"-"` tag so it
 * is never returned in any API response, even if a handler accidentally
 * serialises a User directly.
 */
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`         ///< bcrypt hash — never serialised.
	CreatedAt    time.Time `json:"created_at"`
}

/**
 * @brief Represents a task owned by a single user.
 *
 * DueDate and Tags use pointer / omitempty so they are omitted from JSON
 * when not set, keeping responses clean for simple tasks.
 */
type Task struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      Status     `json:"status"`
	DueDate     *time.Time `json:"due_date,omitempty"` ///< nil means no due date.
	Tags        []string   `json:"tags,omitempty"`     ///< Alphabetically ordered tag names.
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
