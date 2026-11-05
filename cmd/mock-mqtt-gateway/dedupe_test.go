package main

import (
	"path/filepath"
	"testing"
)

func TestMQTTSeenMessagesSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dedupe", "messages.json")
	seen := map[string]bool{"mqtt-light-1:message-1": true}
	if err := saveMQTTSeen(path, seen); err != nil {
		t.Fatalf("save MQTT dedupe store: %v", err)
	}
	recovered, err := loadMQTTSeen(path)
	if err != nil {
		t.Fatalf("load MQTT dedupe store: %v", err)
	}
	if !recovered["mqtt-light-1:message-1"] {
		t.Fatal("expected MQTT message ID to survive gateway restart")
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
