package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fogsense-device-runtime/pkg/gatewaystore"
)

func resetGatewayTestState(t *testing.T, path string) {
	t.Helper()
	if gatewayStore != nil {
		_ = gatewayStore.Close()
		gatewayStore = nil
	}
	mu.Lock()
	devices = make(map[string]*DeviceRecord)
	dedupeFile = path
	deviceToken = ""
	quiet = true
	mu.Unlock()
	store, err := gatewaystore.Open(path)
	if err != nil {
		dedupeHealthy.Store(false)
		return
	}
	gatewayStore = store
	dedupeHealthy.Store(true)
}

func postGatewayJSON(t *testing.T, h http.Handler, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func registerTestDevice(t *testing.T, h http.Handler) {
	t.Helper()
	res := postGatewayJSON(t, h, "/devices/register", map[string]interface{}{
		"deviceId": "light-1",
		"bootId":   "boot-1",
		"endpoint": "http://light-1:8080",
		"status":   "ONLINE",
		"state":    map[string]interface{}{},
		"info": map[string]interface{}{
			"type": "smart-light", "version": 1,
			"telemetry": []interface{}{}, "state": []interface{}{}, "commands": []interface{}{},
		},
	})
	if res.Code != http.StatusOK {
		t.Fatalf("register status = %d, want 200: %s", res.Code, res.Body.String())
	}
}

func telemetryTestMessage(ts string) map[string]interface{} {
	return map[string]interface{}{
		"deviceId": "light-1",
		"bootId": "boot-1",
		"messageId": "message-1",
		"seq": 1,
		"ts": ts,
		"readings": []interface{}{map[string]interface{}{"metric": "temperature", "value": 22.5, "ts": ts}},
	}
}

func TestHTTPGatewayAcknowledgesAndDeduplicatesStableMessageID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway", "gateway.db")
	resetGatewayTestState(t, path)
	defer gatewayStore.Close()
	h := handler()
	registerTestDevice(t, h)

	message := telemetryTestMessage(time.Now().UTC().Format(time.RFC3339Nano))
	first := postGatewayJSON(t, h, "/devices/light-1/telemetry", message)
	if first.Code != http.StatusOK {
		t.Fatalf("first telemetry status = %d, want 200: %s", first.Code, first.Body.String())
	}
	second := postGatewayJSON(t, h, "/devices/light-1/telemetry", message)
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate telemetry status = %d, want 200: %s", second.Code, second.Body.String())
	}
	var duplicate map[string]interface{}
	if err := json.Unmarshal(second.Body.Bytes(), &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate["duplicate"] != true {
		t.Fatalf("duplicate response = %v, want duplicate=true", duplicate)
	}

	mu.Lock()
	rec := devices["light-1"]
	mu.Unlock()
	if rec == nil || rec.TelemetryCount != 1 || rec.Duplicates != 1 {
		t.Fatalf("device record = %+v, want one telemetry and one duplicate", rec)
	}
	stored, err := gatewayStore.Get(context.Background(), "light-1", "message-1")
	if err != nil {
		t.Fatalf("accepted payload was not persisted: %v", err)
	}
	if stored.Kind != "telemetry" || !bytes.Contains(stored.Payload, []byte("temperature")) {
		t.Fatalf("stored message = %+v, want telemetry payload", stored)
	}
}

func TestHTTPGatewayDeduplicationAndPayloadSurviveGatewayRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway", "gateway.db")
	resetGatewayTestState(t, path)
	firstHandler := handler()
	registerTestDevice(t, firstHandler)

	message := telemetryTestMessage(time.Now().UTC().Format(time.RFC3339Nano))
	if res := postGatewayJSON(t, firstHandler, "/devices/light-1/telemetry", message); res.Code != http.StatusOK {
		t.Fatalf("initial telemetry status = %d: %s", res.Code, res.Body.String())
	}

	if err := gatewayStore.Close(); err != nil {
		t.Fatal(err)
	}
	gatewayStore = nil
	devices = make(map[string]*DeviceRecord)
	store, err := gatewaystore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	gatewayStore = store
	dedupeHealthy.Store(true)

	secondHandler := handler()
	registerTestDevice(t, secondHandler)
	replay := postGatewayJSON(t, secondHandler, "/devices/light-1/telemetry", message)
	if replay.Code != http.StatusOK {
		t.Fatalf("replayed telemetry status = %d, want 200: %s", replay.Code, replay.Body.String())
	}
	var response map[string]interface{}
	if err := json.Unmarshal(replay.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["duplicate"] != true {
		t.Fatalf("replay response = %v, want duplicate=true", response)
	}
	stored, err := gatewayStore.Get(context.Background(), "light-1", "message-1")
	if err != nil {
		t.Fatalf("payload missing after restart: %v", err)
	}
	if !bytes.Contains(stored.Payload, []byte("temperature")) {
		t.Fatalf("recovered payload = %s", stored.Payload)
	}
}

func TestHTTPGatewayDoesNotAcknowledgeWhenStoreIsUnavailable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "gateway.db")
	resetGatewayTestState(t, path)
	h := handler()
	registerTestDevice(t, h)

	message := telemetryTestMessage(time.Now().UTC().Format(time.RFC3339Nano))
	for attempt := 0; attempt < 2; attempt++ {
		res := postGatewayJSON(t, h, "/devices/light-1/telemetry", message)
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("attempt %d status = %d, want 503: %s", attempt+1, res.Code, res.Body.String())
		}
	}
	health := httptest.NewRecorder()
	h.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status = %d, want 503 while message storage is unavailable", health.Code)
	}
}
