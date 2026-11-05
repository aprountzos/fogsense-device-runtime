package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetGatewayTestState(path string) {
	mu.Lock()
	defer mu.Unlock()
	devices = make(map[string]*DeviceRecord)
	seenMessages = make(map[string]bool)
	dedupeFile = path
	deviceToken = ""
	dedupeHealthy.Store(true)
	quiet = true
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

func TestHTTPGatewayAcknowledgesAndDeduplicatesStableMessageID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway", "dedupe.json")
	resetGatewayTestState(path)
	h := handler()
	registerTestDevice(t, h)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	message := map[string]interface{}{
		"deviceId": "light-1",
		"bootId":   "boot-1",
		"messageId": "message-1",
		"seq":      1,
		"ts":       ts,
		"readings": []interface{}{map[string]interface{}{"metric": "temperature", "value": 22.5, "ts": ts}},
	}

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
	loaded, err := loadSeenMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded["light-1:message-1"] {
		t.Fatal("accepted message ID was not persisted before HTTP acknowledgement")
	}
}

func TestHTTPGatewayDeduplicationSurvivesGatewayRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway", "dedupe.json")
	resetGatewayTestState(path)
	firstHandler := handler()
	registerTestDevice(t, firstHandler)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	message := map[string]interface{}{
		"deviceId": "light-1", "bootId": "boot-1", "messageId": "message-replayed",
		"seq": 1, "ts": ts,
		"readings": []interface{}{map[string]interface{}{"metric": "temperature", "value": 20.0, "ts": ts}},
	}
	if res := postGatewayJSON(t, firstHandler, "/devices/light-1/telemetry", message); res.Code != http.StatusOK {
		t.Fatalf("initial telemetry status = %d: %s", res.Code, res.Body.String())
	}

	recovered, err := loadSeenMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	devices = make(map[string]*DeviceRecord)
	seenMessages = recovered
	mu.Unlock()

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
}

func TestHTTPGatewayDoesNotAcknowledgeWhenDedupeStoreIsUnavailable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "dedupe.json")
	resetGatewayTestState(path)
	h := handler()
	registerTestDevice(t, h)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	message := map[string]interface{}{
		"deviceId": "light-1", "bootId": "boot-1", "messageId": "message-no-disk",
		"seq": 1, "ts": ts,
		"readings": []interface{}{map[string]interface{}{"metric": "temperature", "value": 20.0, "ts": ts}},
	}
	for attempt := 0; attempt < 2; attempt++ {
		res := postGatewayJSON(t, h, "/devices/light-1/telemetry", message)
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("attempt %d status = %d, want 503: %s", attempt+1, res.Code, res.Body.String())
		}
	}
	health := httptest.NewRecorder()
	h.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status = %d, want 503 while dedupe storage is unavailable", health.Code)
	}
}
