/**
 * @file db.go
 * @package db
 * @brief Database initialisation and schema migration.
 *
 * Opens a SQLite connection, enables foreign-key enforcement, and applies
 * the CREATE TABLE IF NOT EXISTS schema on every startup so the database
 * is always in a known state without a separate migration tool.
 */
package db

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

/// schema holds all DDL statements run on startup.
/// Every statement is idempotent (IF NOT EXISTS) so re-running is safe.
const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	email         TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tasks (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id     INTEGER NOT NULL REFERENCES users(id),
	title       TEXT NOT NULL,
	description TEXT,
	status      TEXT NOT NULL DEFAULT 'todo',
	due_date    DATETIME,
	created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tags (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS task_tags (
	task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	tag_id  INTEGER NOT NULL REFERENCES tags(id),
	PRIMARY KEY (task_id, tag_id)
);
`

/**
 * @brief Opens a SQLite database and applies the schema.
 *
 * Enables PRAGMA foreign_keys so ON DELETE CASCADE on task_tags is enforced,
 * then runs all CREATE TABLE IF NOT EXISTS statements in schema.
 * Pass ":memory:" as dsn for an in-process test database.
 *
 * @param dsn  File path for the SQLite database, or ":memory:".
 * @return     Ready-to-use *sql.DB, or an error if the file cannot be opened
 *             or the schema cannot be applied.
 */
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// Foreign keys are disabled by default in SQLite; enable them so
	// ON DELETE CASCADE on task_tags actually fires.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, err
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return db, nil
}
