package http

import "time"

type Config struct {
	BaseURL                    string
	Path                       string
	Timeout                    time.Duration
	DeviceToken                string
	DeviceMac                  string
	DeviceCodeField            string
	AcceptedFalseIsSuccess     bool
	OverwritePayloadDeviceCode bool
	RetryableStatusCodes       []int
}

type ReportMessage struct {
	Source      string
	DeviceCode  string
	Payload     map[string]any
	CollectedAt int64
	TraceID     string
}

type DeliveryOutcome struct {
	Delivered     bool
	ShouldRetry   bool
	StatusCode    int
	Accepted      *bool
	ResponseBody  []byte
	FailureReason string
}
