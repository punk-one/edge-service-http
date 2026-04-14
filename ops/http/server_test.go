package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
	if got := rec.Header().Get("Content-Type"); got != "application/json" && got != "application/json; charset=utf-8" {
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

func TestServerStatusEndpointReturnsFullStatus(t *testing.T) {
	server := NewServer(StatusProviderFunc(func() Status {
		return Status{
			Healthy:          true,
			Ready:            false,
			PendingCount:     3,
			OldestPendingAge: 99,
			RecentErrors:     []string{"one", "two"},
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var body Status
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.PendingCount != 3 || body.OldestPendingAge != 99 {
		t.Fatalf("unexpected status body: %+v", body)
	}
	if len(body.RecentErrors) != 2 || body.RecentErrors[1] != "two" {
		t.Fatalf("recentErrors = %#v", body.RecentErrors)
	}
}

func TestServerRecentDeliveriesEndpointReturnsErrors(t *testing.T) {
	server := NewServer(StatusProviderFunc(func() Status {
		return Status{
			RecentErrors: []string{"failed-a", "failed-b"},
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/deliveries/recent", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var body struct {
		RecentErrors []string `json:"recentErrors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.RecentErrors) != 2 || body.RecentErrors[0] != "failed-a" {
		t.Fatalf("recentErrors = %#v", body.RecentErrors)
	}
}

func TestServerRegistersCustomRoutes(t *testing.T) {
	server := NewServer(StatusProviderFunc(func() Status {
		return Status{Healthy: true, Ready: true}
	}), func(engine *gin.Engine) {
		engine.GET("/api/v1/custom/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/custom/ping", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
