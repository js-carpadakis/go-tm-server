package testutil

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jcarp/goserver/internal/db"
	"github.com/jcarp/goserver/internal/server"
)

const TestJWTSecret = "test-secret"

func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("testutil: open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func NewTestServer(t *testing.T, database *sql.DB) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(server.New(database, TestJWTSecret))
	t.Cleanup(srv.Close)
	return srv
}

func Post(t *testing.T, srv *httptest.Server, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func Get(t *testing.T, srv *httptest.Server, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func Put(t *testing.T, srv *httptest.Server, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	return resp
}

func Delete(t *testing.T, srv *httptest.Server, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func Decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// AuthHeader registers a fresh user and returns the Bearer token header value.
func AuthHeader(t *testing.T, srv *httptest.Server, email, password string) string {
	t.Helper()
	resp := Post(t, srv, "/auth/register", map[string]string{"email": email, "password": password}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: status %d", resp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	Decode(t, resp, &body)
	return fmt.Sprintf("Bearer %s", body.Token)
}
