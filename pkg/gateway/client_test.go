package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"fogsense-device-runtime/pkg/device"
)

func TestHTTPGatewayRequestUsesDeviceToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("X-Device-Id"); got != "test-device" {
			t.Errorf("X-Device-Id = %q, want test-device", got)
		}
		if got := r.Header.Get("X-Device-Token"); got != "http-secret" {
			t.Errorf("X-Device-Token = %q, want http-secret", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewClient(Config{
		GatewayURL: server.URL,
		Device:     &device.Device{DeviceID: "test-device"},
		Token:      "http-secret",
	})
	status, err := client.post(context.Background(), "/devices/register", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("post() error = %v", err)
	}
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", status, http.StatusAccepted)
	}
}
