package testutil_test

import (
	"net/http"
	"testing"

	"github.com/jcarp/goserver/internal/testutil"
)

func TestRegister(t *testing.T) {
	cases := []struct {
		name   string
		body   map[string]string
		status int
	}{
		{"happy path", map[string]string{"email": "a@example.com", "password": "password123"}, http.StatusCreated},
		{"short password", map[string]string{"email": "b@example.com", "password": "short"}, http.StatusBadRequest},
		{"missing email", map[string]string{"email": "", "password": "password123"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			resp := testutil.Post(t, env.srv, "/auth/register", tc.body, nil)
			if resp.StatusCode != tc.status {
				t.Errorf("got %d, want %d", resp.StatusCode, tc.status)
			}
		})
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	env := newEnv(t)
	body := map[string]string{"email": "dup@example.com", "password": "password123"}
	testutil.Post(t, env.srv, "/auth/register", body, nil)
	resp := testutil.Post(t, env.srv, "/auth/register", body, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("got %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestLogin(t *testing.T) {
	env := newEnv(t)
	testutil.Post(t, env.srv, "/auth/register",
		map[string]string{"email": "user@example.com", "password": "password123"}, nil)

	cases := []struct {
		name   string
		body   map[string]string
		status int
	}{
		{"valid credentials", map[string]string{"email": "user@example.com", "password": "password123"}, http.StatusOK},
		{"wrong password", map[string]string{"email": "user@example.com", "password": "wrongpass"}, http.StatusUnauthorized},
		{"unknown email", map[string]string{"email": "nobody@example.com", "password": "password123"}, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := testutil.Post(t, env.srv, "/auth/login", tc.body, nil)
			if resp.StatusCode != tc.status {
				t.Errorf("got %d, want %d", resp.StatusCode, tc.status)
			}
		})
	}
}

func TestLoginReturnsToken(t *testing.T) {
	env := newEnv(t)
	testutil.Post(t, env.srv, "/auth/register",
		map[string]string{"email": "tok@example.com", "password": "password123"}, nil)
	resp := testutil.Post(t, env.srv, "/auth/login",
		map[string]string{"email": "tok@example.com", "password": "password123"}, nil)
	var body struct {
		Token string `json:"token"`
	}
	testutil.Decode(t, resp, &body)
	if body.Token == "" {
		t.Error("expected non-empty token")
	}
}
