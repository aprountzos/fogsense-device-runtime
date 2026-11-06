package gatewaystore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestAcceptPersistsPayloadAndDeduplicationAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.db")
	ctx := context.Background()
	store, err := Open(path)
	if err != nil { t.Fatal(err) }
	payload := map[string]interface{}{"seq": 7, "readings": []interface{}{map[string]interface{}{"metric":"temperature","value":21.5}}}
	duplicate, err := store.Accept(ctx, "sensor-1", "message-7", "telemetry", payload)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("first delivery was marked duplicate") }
	if err := store.Close(); err != nil { t.Fatal(err) }

	// Reopening simulates a gateway process restart. The payload and dedupe key
	// must be recovered from the same committed row.
	store, err = Open(path)
	if err != nil { t.Fatal(err) }
	defer store.Close()
	duplicate, err = store.Accept(ctx, "sensor-1", "message-7", "telemetry", map[string]string{"different":"payload"})
	if err != nil { t.Fatal(err) }
	if !duplicate { t.Fatal("replayed message was not marked duplicate") }
	got, err := store.Get(ctx, "sensor-1", "message-7")
	if err != nil { t.Fatal(err) }
	if got.Kind != "telemetry" { t.Fatalf("kind = %q", got.Kind) }
	var decoded map[string]interface{}
	if err := json.Unmarshal(got.Payload, &decoded); err != nil { t.Fatal(err) }
	if decoded["seq"] != float64(7) { t.Fatalf("persisted payload = %s", got.Payload) }
	count, err := store.Count(ctx)
	if err != nil { t.Fatal(err) }
	if count != 1 { t.Fatalf("accepted message count = %d, want 1", count) }
}

func TestAcceptRequiresStableIdentity(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil { t.Fatal(err) }
	defer store.Close()
	if _, err := store.Accept(context.Background(), "sensor-1", "", "telemetry", map[string]int{"seq":1}); err != ErrInvalidMessage {
		t.Fatalf("error = %v, want ErrInvalidMessage", err)
	}
}
