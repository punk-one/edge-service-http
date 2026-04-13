package reporting

import (
    "context"
    "encoding/json"
    "errors"
    "testing"

    "github.com/punk-one/edge-service-http/reliable"
)

type fakeDispatcher struct {
    called      bool
    submitErr   error
    lastMessage reliable.OutboundMessage
}

func (f *fakeDispatcher) Submit(_ context.Context, msg reliable.OutboundMessage) error {
    f.called = true
    f.lastMessage = msg
    return f.submitErr
}

func TestReporterSubmitPassesStructuredMessage(t *testing.T) {
    dispatcher := &fakeDispatcher{}
    reporter := New(dispatcher)

    err := reporter.Report(context.Background(), Message{
        Source:     "spectrum",
        DeviceCode: "device-01",
        TraceID:    "trace-001",
        Payload: map[string]any{
            "sampleId": "S-001",
        },
    })
    if err != nil {
        t.Fatalf("Report returned error: %v", err)
    }
    if dispatcher.lastMessage.DeviceCode != "device-01" {
        t.Fatalf("deviceCode = %q", dispatcher.lastMessage.DeviceCode)
    }
    expectedPayloadJSON, err := json.Marshal(map[string]any{
        "sampleId": "S-001",
    })
    if err != nil {
        t.Fatalf("marshal payload: %v", err)
    }
    if string(dispatcher.lastMessage.PayloadJSON) != string(expectedPayloadJSON) {
        t.Fatalf("payloadJSON = %s", string(dispatcher.lastMessage.PayloadJSON))
    }
    if dispatcher.lastMessage.CollectedAt == 0 {
        t.Fatal("collectedAt was not defaulted")
    }
    if dispatcher.lastMessage.TraceID != "trace-001" {
        t.Fatalf("traceID = %q", dispatcher.lastMessage.TraceID)
    }
}

func TestReporterSubmitRejectsEmptyDeviceCode(t *testing.T) {
    dispatcher := &fakeDispatcher{}
    reporter := New(dispatcher)

    err := reporter.Report(context.Background(), Message{
        Source:     "spectrum",
        DeviceCode: "",
        Payload: map[string]any{
            "sampleId": "S-001",
        },
    })
    if err == nil {
        t.Fatal("Report returned nil error")
    }
    if dispatcher.called {
        t.Fatal("dispatcher.Submit was called")
    }
}

func TestReporterSubmitForwardsDispatcherError(t *testing.T) {
    submitErr := errors.New("submit failed")
    dispatcher := &fakeDispatcher{submitErr: submitErr}
    reporter := New(dispatcher)

    err := reporter.Report(context.Background(), Message{
        Source:     "spectrum",
        DeviceCode: "device-01",
        Payload: map[string]any{
            "sampleId": "S-001",
        },
    })
    if err == nil {
        t.Fatal("Report returned nil error")
    }
    if !errors.Is(err, submitErr) {
        t.Fatalf("Report error = %v", err)
    }
    if !dispatcher.called {
        t.Fatal("dispatcher.Submit was not called")
    }
}
