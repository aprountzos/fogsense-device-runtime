package gateway

import (
	"net/http"
	"net/http/httptest"
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

func TestHTTPApplicationAckRemovesPersistedOutboxMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewClient(Config{
		GatewayURL: server.URL,
		Device:     &device.Device{DeviceID: "light-1", BootID: "boot-one"},
		OutboxPath: path,
	})
	client.enqueue("/devices/light-1/events", map[string]interface{}{"type": "STATUS_CHANGED"})
	client.registered = true
	client.doFlush()

	if len(client.queue) != 0 {
		t.Fatalf("queued messages after HTTP acceptance = %d, want 0", len(client.queue))
	}
	state, err := loadOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Queue) != 0 {
		t.Fatalf("persisted queue after HTTP acceptance = %d, want 0", len(state.Queue))
	}
}

func TestTransientHTTPStatusesKeepOutboxMessage(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "outbox.json")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			client := NewClient(Config{
				GatewayURL: server.URL,
				Device:     &device.Device{DeviceID: "light-1", BootID: "boot-one"},
				OutboxPath: path,
			})
			client.enqueue("/devices/light-1/events", map[string]interface{}{"type": "STATUS_CHANGED"})
			client.registered = true
			client.doFlush()
			if len(client.queue) != 1 {
				t.Fatalf("queued messages after transient HTTP %d = %d, want 1", status, len(client.queue))
			}
		})
	}
}

func TestPermanentHTTPRejectionIsCountedAndRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(Config{
		GatewayURL: server.URL,
		Device:     &device.Device{DeviceID: "light-1", BootID: "boot-one"},
		OutboxPath: path,
	})
	client.enqueue("/devices/light-1/events", map[string]interface{}{"type": "STATUS_CHANGED"})
	client.registered = true
	client.doFlush()

	if len(client.queue) != 0 {
		t.Fatalf("queued messages after permanent rejection = %d, want 0", len(client.queue))
	}
	if client.dropped != 1 {
		t.Fatalf("dropped = %d, want 1", client.dropped)
	}
	state, err := loadOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Dropped != 1 || len(state.Queue) != 0 {
		t.Fatalf("persisted outbox = %+v, want dropped=1 and empty queue", state)
	}
}

func TestOutboxRejectsNewestMessageWhenFull(t *testing.T) {
	client := NewClient(Config{Device: &device.Device{DeviceID: "light-1", BootID: "boot-one"}})
	for i := 0; i < MaxQueue+1; i++ {
		client.enqueue("/devices/light-1/events", map[string]interface{}{"index": i})
	}
	if len(client.queue) != MaxQueue {
		t.Fatalf("queue length = %d, want %d", len(client.queue), MaxQueue)
	}
	if client.dropped != 1 {
		t.Fatalf("dropped = %d, want 1", client.dropped)
	}
	if client.seq != MaxQueue {
		t.Fatalf("sequence = %d, want %d because rejected messages are not assigned a sequence", client.seq, MaxQueue)
	}
	if got := client.queue[0].Body["seq"]; got != int64(1) {
		t.Fatalf("oldest queued sequence = %v, want 1", got)
	}
}

func TestClientInitializationFailsOnCorruptOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	client := NewClient(Config{
		Device:    &device.Device{DeviceID: "light-1", BootID: "boot-one"},
		OutboxPath: path,
	})
	if client.InitializationError() == nil {
		t.Fatal("expected corrupt persisted outbox to prevent startup")
	}
	if client.outboxHealthy {
		t.Fatal("outbox health must be false when recovery fails")
	}
}

func TestClientInitializationCreatesOutboxFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "outbox.json")
	client := NewClient(Config{
		Device:    &device.Device{DeviceID: "light-1", BootID: "boot-one"},
		OutboxPath: path,
	})
	if err := client.InitializationError(); err != nil {
		t.Fatalf("initialize outbox: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("outbox file was not created: %v", err)
	}
	if info.IsDir() {
		t.Fatal("outbox path is a directory, want file")
	}
}

func TestTransientFailureSchedulesBackoffAndSuccessClearsIt(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewClient(Config{
		GatewayURL: server.URL,
		Device:     &device.Device{DeviceID: "light-1", BootID: "boot-one"},
	})
	client.enqueue("/devices/light-1/events", map[string]interface{}{"type": "STATUS_CHANGED"})
	client.registered = true
	client.doFlush()

	failed := client.Stats()
	if failed.FlushFailures != 1 || failed.NextRetryAt == "" {
		t.Fatalf("stats after transient failure = %+v, want one failure and a scheduled retry", failed)
	}

	client.doFlush()
	recovered := client.Stats()
	if recovered.FlushFailures != 0 || recovered.NextRetryAt != "" || recovered.Queued != 0 {
		t.Fatalf("stats after successful retry = %+v, want empty queue and cleared backoff", recovered)
	}
}
