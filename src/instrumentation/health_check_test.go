package instrumentation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestHealthCheckApi_GetHealth(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
healthy: true
status: "not ready"
status_time_unix_nano: 1734559668210947000
start_time_unix_nano:  1734559614000000000
agent_run_id: "1234-56"
last_error: "some error"
`))
	}))
	defer testServer.Close()
	healthChecker := NewHealthCheckApi(nil)
	health, _ := healthChecker.GetHealth(context.Background(), testServer.URL)
	diff := cmp.Diff(health, Health{LastError: "some error", Status: "not ready", Healthy: true, StatusTime: 1734559668210947000, StartTime: 1734559614000000000, AgentRunID: "1234-56"})
	if diff != "" {
		t.Fatal(diff)
	}
}
