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
