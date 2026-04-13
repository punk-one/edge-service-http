package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/punk-one/edge-service-http/reliable"
)

// Message represents a structured device report.
type Message struct {
	Source      string
	DeviceCode  string
	Payload     map[string]any
	CollectedAt int64
	TraceID     string
}

type dispatcher interface {
	Submit(context.Context, reliable.OutboundMessage) error
}

// Service sends reports through a dispatcher.
type Service struct {
	dispatcher dispatcher
}

func New(dispatcher dispatcher) *Service {
	return &Service{dispatcher: dispatcher}
}

func (s *Service) Report(ctx context.Context, message Message) error {
	if strings.TrimSpace(message.DeviceCode) == "" {
		return fmt.Errorf("deviceCode is required")
	}
	payloadJSON, err := json.Marshal(message.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	if message.CollectedAt == 0 {
		message.CollectedAt = time.Now().UnixMilli()
	}
	return s.dispatcher.Submit(ctx, reliable.OutboundMessage{
		Source:      message.Source,
		DeviceCode:  message.DeviceCode,
		PayloadJSON: payloadJSON,
		CollectedAt: message.CollectedAt,
		TraceID:     message.TraceID,
	})
}
