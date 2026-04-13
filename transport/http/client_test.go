package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientInjectsHeadersAndDeviceCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/external/iot/spectrum" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Device-Token"); got != "token-1" {
			t.Fatalf("token header = %q", got)
		}
		if got := r.Header.Get("X-Device-Mac"); got != "AA:BB" {
			t.Fatalf("mac header = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got := body["deviceCode"]; got != "device-01" {
			t.Fatalf("deviceCode = %v", got)
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","data":{"accepted":true}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:                server.URL,
		Path:                   "/api/external/iot/spectrum",
		Timeout:                2 * time.Second,
		DeviceToken:            "token-1",
		DeviceMac:              "AA:BB",
		DeviceCodeField:        "deviceCode",
		AcceptedFalseIsSuccess: true,
		RetryableStatusCodes:   []int{408, 429, 500, 502, 503, 504},
	}, nil)

	outcome, err := client.Send(context.Background(), ReportMessage{
		Source:     "spectrum",
		DeviceCode: "device-01",
		Payload: map[string]any{
			"sampleId": "S-001",
		},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !outcome.Delivered || outcome.ShouldRetry {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestClientTreatsAcceptedFalseAsDelivered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","data":{"accepted":false}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Path: "/push", Timeout: time.Second, DeviceCodeField: "deviceCode", AcceptedFalseIsSuccess: true}, nil)
	outcome, err := client.Send(context.Background(), ReportMessage{DeviceCode: "device-01", Payload: map[string]any{"sampleId": "S-002"}})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !outcome.Delivered {
		t.Fatalf("accepted=false should be delivered: %+v", outcome)
	}
}

func TestClientMarksHTTP503Retryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Path: "/push", Timeout: time.Second, DeviceCodeField: "deviceCode", RetryableStatusCodes: []int{503}}, nil)
	outcome, err := client.Send(context.Background(), ReportMessage{DeviceCode: "device-01", Payload: map[string]any{"sampleId": "S-003"}})
	if err == nil {
		t.Fatalf("expected error for 503")
	}
	if !outcome.ShouldRetry {
		t.Fatalf("503 should be retryable: %+v", outcome)
	}
}

func TestBuildPayloadRequiresDeviceCode(t *testing.T) {
	client := NewClient(Config{DeviceCodeField: "deviceCode"}, nil)

	_, err := client.buildPayload(ReportMessage{
		DeviceCode: "",
		Payload:    map[string]any{"sampleId": "S-004"},
	})
	if err == nil {
		t.Fatalf("expected error for blank deviceCode")
	}
}

func TestClientReadFailureMarksRetryable(t *testing.T) {
	client := NewClient(Config{
		BaseURL:              "http://example.com",
		Path:                 "/push",
		Timeout:              time.Second,
		DeviceCodeField:      "deviceCode",
		RetryableStatusCodes: []int{429},
	}, nil)
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(errReader{}),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	outcome, err := client.Send(context.Background(), ReportMessage{
		DeviceCode: "device-01",
		Payload:    map[string]any{"sampleId": "S-005"},
	})
	if err == nil {
		t.Fatalf("expected read error")
	}
	if !outcome.ShouldRetry {
		t.Fatalf("read error on 503 should be retryable: %+v", outcome)
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read failed")
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
