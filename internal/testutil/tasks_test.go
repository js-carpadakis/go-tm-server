package testutil_test

import (
	"net/http"
	"testing"

	"github.com/jcarp/goserver/internal/testutil"
)

func TestTaskCRUD(t *testing.T) {
	env := newEnv(t)
	headers := env.auth(t, "user@example.com", "password123")

	// Create
	resp := testutil.Post(t, env.srv, "/tasks",
		map[string]any{"title": "Buy milk", "status": "todo"}, headers)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d", resp.StatusCode)
	}

	// Get
	resp = testutil.Get(t, env.srv, "/tasks/1", headers)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: got %d", resp.StatusCode)
	}

	// Update
	resp = testutil.Put(t, env.srv, "/tasks/1",
		map[string]any{"status": "done"}, headers)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: got %d", resp.StatusCode)
	}
	var updated map[string]any
	testutil.Decode(t, resp, &updated)
	if updated["status"] != "done" {
		t.Errorf("status not updated: got %v", updated["status"])
	}

	// Delete
	resp = testutil.Delete(t, env.srv, "/tasks/1", headers)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d", resp.StatusCode)
	}

	// Get after delete → 404
	resp = testutil.Get(t, env.srv, "/tasks/1", headers)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("after delete: got %d, want 404", resp.StatusCode)
	}
}

func TestTasksRequireAuth(t *testing.T) {
	env := newEnv(t)
	resp := testutil.Get(t, env.srv, "/tasks", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
}

func TestOwnershipIsolation(t *testing.T) {
	env := newEnv(t)
	authA := env.auth(t, "alice@example.com", "password123")
	authB := env.auth(t, "bob@example.com", "password123")

	// Alice creates a task
	testutil.Post(t, env.srv, "/tasks", map[string]any{"title": "Alice task"}, authA)

	// Bob cannot get Alice's task
	resp := testutil.Get(t, env.srv, "/tasks/1", authB)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("bob got alice's task: status %d", resp.StatusCode)
	}

	// Bob's list is empty
	resp = testutil.Get(t, env.srv, "/tasks", authB)
	var tasks []any
	testutil.Decode(t, resp, &tasks)
	if len(tasks) != 0 {
		t.Errorf("bob sees %d tasks, want 0", len(tasks))
	}
}

func TestFilterByStatus(t *testing.T) {
	env := newEnv(t)
	headers := env.auth(t, "user@example.com", "password123")

	testutil.Post(t, env.srv, "/tasks", map[string]any{"title": "Task A", "status": "todo"}, headers)
	testutil.Post(t, env.srv, "/tasks", map[string]any{"title": "Task B", "status": "done"}, headers)

	resp := testutil.Get(t, env.srv, "/tasks?status=todo", headers)
	var tasks []map[string]any
	testutil.Decode(t, resp, &tasks)
	if len(tasks) != 1 {
		t.Errorf("got %d tasks for status=todo, want 1", len(tasks))
	}
	if tasks[0]["title"] != "Task A" {
		t.Errorf("unexpected task: %v", tasks[0]["title"])
	}
}

func TestFilterByTag(t *testing.T) {
	env := newEnv(t)
	headers := env.auth(t, "user@example.com", "password123")

	testutil.Post(t, env.srv, "/tasks",
		map[string]any{"title": "Tagged", "tags": []string{"work"}}, headers)
	testutil.Post(t, env.srv, "/tasks",
		map[string]any{"title": "Untagged"}, headers)

	resp := testutil.Get(t, env.srv, "/tasks?tag=work", headers)
	var tasks []map[string]any
	testutil.Decode(t, resp, &tasks)
	if len(tasks) != 1 {
		t.Errorf("got %d tasks for tag=work, want 1", len(tasks))
	}
}

func TestAddAndRemoveTag(t *testing.T) {
	env := newEnv(t)
	headers := env.auth(t, "user@example.com", "password123")

	testutil.Post(t, env.srv, "/tasks", map[string]any{"title": "My task"}, headers)

	// Add tag
	resp := testutil.Post(t, env.srv, "/tasks/1/tags",
		map[string]any{"tags": []string{"urgent"}}, headers)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add tag: got %d", resp.StatusCode)
	}
	var task map[string]any
	testutil.Decode(t, resp, &task)
	tags := task["tags"].([]any)
	if len(tags) != 1 || tags[0] != "urgent" {
		t.Errorf("expected [urgent], got %v", tags)
	}

	// Remove tag
	resp = testutil.Delete(t, env.srv, "/tasks/1/tags/urgent", headers)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove tag: got %d", resp.StatusCode)
	}
	var task2 map[string]any
	testutil.Decode(t, resp, &task2)
	if task2["tags"] != nil && len(task2["tags"].([]any)) != 0 {
		t.Errorf("expected no tags, got %v", task2["tags"])
	}
}

func TestInvalidStatus(t *testing.T) {
	env := newEnv(t)
	headers := env.auth(t, "user@example.com", "password123")

	resp := testutil.Post(t, env.srv, "/tasks",
		map[string]any{"title": "Task", "status": "invalid"}, headers)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}
