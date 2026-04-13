package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerQueueEndpointReturnsPendingCount(t *testing.T) {
	server := NewServer(StatusProviderFunc(func() Status {
		return Status{
			Healthy:          true,
			Ready:            true,
			PendingCount:     7,
			OldestPendingAge: 1250,
			RecentErrors:     []string{"boom"},
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/queue", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got := body["pendingCount"]; got != float64(7) {
		t.Fatalf("pendingCount = %v", got)
	}
}

func TestServerHealthAndReadyReflectStatus(t *testing.T) {
	server := NewServer(StatusProviderFunc(func() Status {
		return Status{Healthy: false, Ready: true}
	}))

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	healthRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status = %d", healthRec.Code)
	}

	readyReq := httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil)
	readyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusOK {
		t.Fatalf("ready status = %d", readyRec.Code)
	}
}
