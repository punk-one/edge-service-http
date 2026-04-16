package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"slices"
	"strings"
	"time"

	"github.com/punk-one/edge-service-http/logging"
)

const (
	defaultTimeout         = 15 * time.Second
	defaultDeviceCodeField = "deviceCode"
)

type Client struct {
	cfg        Config
	httpClient *stdhttp.Client
	logger     logging.Logger
}

func NewClient(cfg Config, logger logging.Logger) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if strings.TrimSpace(cfg.DeviceCodeField) == "" {
		cfg.DeviceCodeField = defaultDeviceCodeField
	}
	if logger == nil {
		logger = logging.New("info", "json")
	}

	return &Client{
		cfg:        cfg,
		httpClient: &stdhttp.Client{Timeout: cfg.Timeout},
		logger:     logger,
	}
}

func (c *Client) Send(ctx context.Context, msg ReportMessage) (DeliveryOutcome, error) {
	payload, err := c.buildPayload(msg)
	if err != nil {
		return DeliveryOutcome{FailureReason: err.Error()}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return DeliveryOutcome{FailureReason: err.Error()}, err
	}

	endpoint := strings.TrimSpace(c.cfg.URL)
	c.logger.Debug("http transport sending request", "endpoint", endpoint, "source", msg.Source)

	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return DeliveryOutcome{FailureReason: err.Error()}, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.cfg.DeviceToken); token != "" {
		req.Header.Set("X-Device-Token", token)
	}
	if mac := strings.TrimSpace(c.cfg.DeviceMac); mac != "" {
		req.Header.Set("X-Device-Mac", mac)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		shouldRetry := true
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			shouldRetry = false
		}
		c.logger.Warn("http transport request failed", "endpoint", endpoint, "retry", shouldRetry, "error", err)
		return DeliveryOutcome{ShouldRetry: shouldRetry, FailureReason: err.Error()}, err
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		shouldRetry := resp.StatusCode >= 500 || slices.Contains(c.cfg.RetryableStatusCodes, resp.StatusCode)
		c.logger.Warn("http transport response read failed", "status", resp.StatusCode, "retry", shouldRetry, "error", readErr)
		return DeliveryOutcome{
			StatusCode:    resp.StatusCode,
			ShouldRetry:   shouldRetry,
			FailureReason: readErr.Error(),
		}, readErr
	}

	outcome := DeliveryOutcome{StatusCode: resp.StatusCode, ResponseBody: respBody}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		outcome.ShouldRetry = slices.Contains(c.cfg.RetryableStatusCodes, resp.StatusCode)
		outcome.FailureReason = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		c.logger.Warn("http transport returned non-success status", "status", resp.StatusCode, "retry", outcome.ShouldRetry)
		return outcome, errors.New(outcome.FailureReason)
	}

	accepted := parseAccepted(respBody)
	if accepted != nil {
		outcome.Accepted = accepted
		if !*accepted && !c.cfg.AcceptedFalseIsSuccess {
			outcome.FailureReason = "response accepted=false"
			c.logger.Warn("http transport rejected by downstream", "status", resp.StatusCode)
			return outcome, errors.New(outcome.FailureReason)
		}
	}

	outcome.Delivered = true
	c.logger.Debug("http transport delivered", "status", resp.StatusCode)
	return outcome, nil
}

func (c *Client) SendRaw(ctx context.Context, payloadJSON []byte, deviceCode string) (DeliveryOutcome, error) {
	payload, err := decodePayload(payloadJSON)
	if err != nil {
		return DeliveryOutcome{FailureReason: err.Error()}, err
	}
	return c.Send(ctx, ReportMessage{DeviceCode: deviceCode, Payload: payload})
}

func (c *Client) buildPayload(msg ReportMessage) (map[string]any, error) {
	payload := make(map[string]any, len(msg.Payload)+1)
	for k, v := range msg.Payload {
		payload[k] = v
	}

	deviceCodeField := c.cfg.DeviceCodeField
	if strings.TrimSpace(deviceCodeField) == "" {
		deviceCodeField = defaultDeviceCodeField
	}

	if strings.TrimSpace(msg.DeviceCode) == "" {
		return nil, fmt.Errorf("%s is required", deviceCodeField)
	}

	if existing, exists := payload[deviceCodeField]; exists {
		if existingCode, ok := existing.(string); !ok || existingCode != msg.DeviceCode {
			if !c.cfg.OverwritePayloadDeviceCode {
				return nil, fmt.Errorf("payload %q conflicts with explicit DeviceCode", deviceCodeField)
			}
		}
	}

	if c.cfg.OverwritePayloadDeviceCode || payload[deviceCodeField] == nil {
		payload[deviceCodeField] = msg.DeviceCode
	}

	return payload, nil
}

func decodePayload(raw []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func parseAccepted(body []byte) *bool {
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}

	if accepted, ok := decoded["accepted"].(bool); ok {
		return &accepted
	}

	data, ok := decoded["data"].(map[string]any)
	if !ok {
		return nil
	}

	accepted, ok := data["accepted"].(bool)
	if !ok {
		return nil
	}
	return &accepted
}
