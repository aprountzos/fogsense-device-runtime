package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"fogsense-device-runtime/pkg/gatewaystore"
)

func TestMQTTAcceptedPayloadAndDeduplicationSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway", "mqtt-gateway.db")
	ctx := context.Background()
	store, err := gatewaystore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	payload := map[string]interface{}{
		"deviceId": "mqtt-light-1",
		"bootId": "boot-1",
		"messageId": "message-1",
		"seq": float64(1),
		"ts": "2026-11-06T00:00:00Z",
		"readings": []interface{}{map[string]interface{}{"metric": "temperature", "value": 20.5}},
	}
	deviceID, messageID, kind, duplicate, err := acceptMQTTDelivery(ctx, store, payload, "fogsense/v1/devices/mqtt-light-1/telemetry", "fogsense/v1/devices")
	if err != nil {
		t.Fatal(err)
	}
	if deviceID != "mqtt-light-1" || messageID != "message-1" || kind != "telemetry" {
		t.Fatalf("accepted identity = %q/%q kind=%q", deviceID, messageID, kind)
	}
	if duplicate {
		t.Fatal("first MQTT delivery was marked duplicate")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = gatewaystore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _, _, duplicate, err = acceptMQTTDelivery(ctx, store, payload, "fogsense/v1/devices/mqtt-light-1/telemetry", "fogsense/v1/devices")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("replayed MQTT message was not marked duplicate")
	}
	stored, err := store.Get(ctx, "mqtt-light-1", "message-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Kind != "telemetry" {
		t.Fatalf("stored kind = %q, want telemetry", stored.Kind)
	}
	var recovered map[string]interface{}
	if err := json.Unmarshal(stored.Payload, &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered["deviceId"] != "mqtt-light-1" || recovered["messageId"] != "message-1" {
		t.Fatalf("recovered payload = %v", recovered)
	}
}

func TestValidateMQTTDeliveryMessage(t *testing.T) {
	payload := map[string]interface{}{
		"deviceId": "light-1",
		"bootId": "boot-1",
		"messageId": "message-1",
		"seq": float64(7),
		"ts": "2026-11-06T00:00:00Z",
	}
	deviceID, messageID, reason := validateMQTTDeliveryMessage(payload, "fogsense/v1/devices/light-1/telemetry", "fogsense/v1/devices")
	if deviceID != "light-1" || messageID != "message-1" || reason != "" {
		t.Fatalf("validation = (%q, %q, %q), want valid light-1/message-1", deviceID, messageID, reason)
	}
}

func TestValidateMQTTDeliveryMessageRejectsTopicIdentityMismatch(t *testing.T) {
	payload := map[string]interface{}{
		"deviceId": "sensor-1",
		"bootId": "boot-1",
		"messageId": "message-1",
		"seq": float64(7),
		"ts": "2026-11-06T00:00:00Z",
	}
	deviceID, messageID, reason := validateMQTTDeliveryMessage(payload, "fogsense/v1/devices/light-1/telemetry", "fogsense/v1/devices")
	if deviceID != "light-1" || messageID != "message-1" || reason == "" {
		t.Fatalf("validation = (%q, %q, %q), want topic device light-1 and a rejection reason", deviceID, messageID, reason)
	}
}
