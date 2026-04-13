package reporting

import (
    "context"
    "testing"

    "github.com/punk-one/edge-service-http/reliable"
)

type fakeDispatcher struct {
    lastMessage reliable.OutboundMessage
}

func (f *fakeDispatcher) Submit(_ context.Context, msg reliable.OutboundMessage) error {
    f.lastMessage = msg
    return nil
}

func TestReporterSubmitPassesStructuredMessage(t *testing.T) {
    dispatcher := &fakeDispatcher{}
    reporter := New(dispatcher)

    err := reporter.Report(context.Background(), Message{
        Source:     "spectrum",
        DeviceCode: "device-01",
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
}
