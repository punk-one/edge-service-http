package http

import (
	"encoding/json"
	"net/http"
)

type Status struct {
	Healthy          bool     `json:"healthy"`
	Ready            bool     `json:"ready"`
	PendingCount     int64    `json:"pendingCount"`
	OldestPendingAge int64    `json:"oldestPendingAgeMs"`
	RecentErrors     []string `json:"recentErrors"`
}

type StatusProvider interface {
	Status() Status
}

type StatusProviderFunc func() Status

func (f StatusProviderFunc) Status() Status {
	return f()
}

type Server struct {
	provider StatusProvider
}

func NewServer(provider StatusProvider) *Server {
	if provider == nil {
		provider = StatusProviderFunc(func() Status { return Status{} })
	}
	return &Server{provider: provider}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		status := s.provider.Status()
		code := http.StatusOK
		if !status.Healthy {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]bool{"healthy": status.Healthy})
	})
	mux.HandleFunc("/api/v1/ready", func(w http.ResponseWriter, r *http.Request) {
		status := s.provider.Status()
		code := http.StatusOK
		if !status.Ready {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]bool{"ready": status.Ready})
	})
	mux.HandleFunc("/api/v1/runtime/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.provider.Status())
	})
	mux.HandleFunc("/api/v1/runtime/queue", func(w http.ResponseWriter, r *http.Request) {
		status := s.provider.Status()
		writeJSON(w, http.StatusOK, map[string]int64{
			"pendingCount":       status.PendingCount,
			"oldestPendingAgeMs": status.OldestPendingAge,
		})
	})
	mux.HandleFunc("/api/v1/runtime/deliveries/recent", func(w http.ResponseWriter, r *http.Request) {
		status := s.provider.Status()
		writeJSON(w, http.StatusOK, map[string][]string{
			"recentErrors": append([]string(nil), status.RecentErrors...),
		})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
