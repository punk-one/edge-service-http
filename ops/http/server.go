package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

type RouteRegistrar func(*gin.Engine)

type Server struct {
	provider   StatusProvider
	registrars []RouteRegistrar
}

func NewServer(provider StatusProvider, registrars ...RouteRegistrar) *Server {
	if provider == nil {
		provider = StatusProviderFunc(func() Status { return Status{} })
	}
	return &Server{
		provider:   provider,
		registrars: append([]RouteRegistrar(nil), registrars...),
	}
}

func (s *Server) Handler() http.Handler {
	engine := gin.New()
	engine.Any("/api/v1/health", s.handleHealth)
	engine.Any("/api/v1/ready", s.handleReady)
	engine.Any("/api/v1/runtime/status", s.handleRuntimeStatus)
	engine.Any("/api/v1/runtime/queue", s.handleRuntimeQueue)
	engine.Any("/api/v1/runtime/deliveries/recent", s.handleRecentDeliveries)
	for _, register := range s.registrars {
		if register != nil {
			register(engine)
		}
	}
	return engine
}

func (s *Server) handleHealth(c *gin.Context) {
	status := s.provider.Status()
	code := http.StatusOK
	if !status.Healthy {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, map[string]bool{"healthy": status.Healthy})
}

func (s *Server) handleReady(c *gin.Context) {
	status := s.provider.Status()
	code := http.StatusOK
	if !status.Ready {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, map[string]bool{"ready": status.Ready})
}

func (s *Server) handleRuntimeStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.provider.Status())
}

func (s *Server) handleRuntimeQueue(c *gin.Context) {
	status := s.provider.Status()
	c.JSON(http.StatusOK, map[string]int64{
		"pendingCount":       status.PendingCount,
		"oldestPendingAgeMs": status.OldestPendingAge,
	})
}

func (s *Server) handleRecentDeliveries(c *gin.Context) {
	status := s.provider.Status()
	c.JSON(http.StatusOK, map[string][]string{
		"recentErrors": append([]string(nil), status.RecentErrors...),
	})
}
