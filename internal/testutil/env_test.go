package testutil_test

import (
	"net/http/httptest"
	"testing"

	"github.com/jcarp/goserver/internal/testutil"
)

// testEnv holds the per-test server and provides convenience helpers.
// Create one with newEnv(t); cleanup is registered automatically via t.Cleanup.
type testEnv struct {
	srv *httptest.Server
}

// newEnv spins up a fresh in-memory database and test server for one test.
func newEnv(t *testing.T) *testEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	srv := testutil.NewTestServer(t, db)
	return &testEnv{srv: srv}
}

// auth registers a user and returns a header map with the Bearer token set.
// Fails the test immediately if registration does not return 201.
func (e *testEnv) auth(t *testing.T, email, password string) map[string]string {
	t.Helper()
	return map[string]string{
		"Authorization": testutil.AuthHeader(t, e.srv, email, password),
	}
}
