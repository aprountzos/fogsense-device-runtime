package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"fogsense-device-runtime/pkg/device"
)

func TestOutboxPersistsAndRecoversMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device", "outbox.json")
	firstDevice := &device.Device{DeviceID: "light-1", BootID: "boot-one"}
	first := NewClient(Config{Device: firstDevice, OutboxPath: path})
	first.enqueue("/devices/light-1/events", map[string]interface{}{"type": "STATUS_CHANGED"})

	if len(first.queue) != 1 {
		t.Fatalf("queued messages = %d, want 1", len(first.queue))
	}
	messageID := first.queue[0].MessageID
	if messageID == "" {
		t.Fatal("messageId must be assigned before persistence")
	}

	secondDevice := &device.Device{DeviceID: "light-1", BootID: "boot-two"}
	recovered := NewClient(Config{Device: secondDevice, OutboxPath: path})
	if recovered.outboxErr != nil {
		t.Fatalf("recover outbox: %v", recovered.outboxErr)
	}
	if len(recovered.queue) != 1 {
		t.Fatalf("recovered messages = %d, want 1", len(recovered.queue))
	}
	if recovered.queue[0].MessageID != messageID {
		t.Fatalf("recovered messageId = %q, want stable ID %q", recovered.queue[0].MessageID, messageID)
	}
	if got := recovered.queue[0].Body["bootId"]; got != "boot-two" {
		t.Fatalf("recovered bootId = %v, want current boot-two", got)
	}
	if recovered.seq != 1 {
		t.Fatalf("recovered sequence = %d, want 1", recovered.seq)
	}
}

func TestOutboxRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOutbox(path); err == nil {
		t.Fatal("expected corrupt outbox to fail recovery")
	}
}

func TestOutboxRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"seq":0,"queue":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOutbox(path); err == nil {
		t.Fatal("expected unsupported outbox version to fail recovery")
	}
}

func TestOutboxAcknowledgementIsPersistedBeforeRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	state := outboxState{
		Version: 1,
		Seq:     1,
		Queue: []OutboundMessage{{
			MessageID: "message-1",
			Path:      "/devices/light-1/events",
			Body:      map[string]interface{}{"deviceId": "light-1", "messageId": "message-1"},
		}},
	}
	if err := saveOutbox(path, state); err != nil {
		t.Fatal(err)
	}
	next := state
	next.Queue = []OutboundMessage{}
	if err := saveOutbox(path, next); err != nil {
		t.Fatal(err)
	}
	recovered, err := loadOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.Queue) != 0 {
		t.Fatalf("recovered queue length = %d, want 0", len(recovered.Queue))
	}
}
