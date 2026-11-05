package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type outboxState struct {
	Version int               `json:"version"`
	Seq     int64             `json:"seq"`
	Dropped int64             `json:"dropped"`
	Queue   []OutboundMessage `json:"queue"`
}

func loadOutbox(path string) (outboxState, error) {
	state := outboxState{Version: 1, Queue: make([]OutboundMessage, 0, MaxQueue)}
	if path == "" {
		return state, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read outbox %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode outbox %q: %w", path, err)
	}
	if state.Version != 1 {
		return state, fmt.Errorf("unsupported outbox version %d", state.Version)
	}
	if len(state.Queue) > MaxQueue {
		return state, fmt.Errorf("outbox contains %d messages; maximum is %d", len(state.Queue), MaxQueue)
	}
	if state.Queue == nil {
		state.Queue = make([]OutboundMessage, 0, MaxQueue)
	}
	for i, msg := range state.Queue {
		if msg.MessageID == "" || msg.Path == "" || msg.Body == nil {
			return state, fmt.Errorf("outbox message %d is missing messageId, path or body", i)
		}
		bodyMessageID, ok := msg.Body["messageId"].(string)
		if !ok || bodyMessageID != msg.MessageID {
			return state, fmt.Errorf("outbox message %d has inconsistent messageId", i)
		}
	}
	return state, nil
}

// saveOutbox replaces the state atomically. The temporary file is created in
// the same directory so rename stays on the same filesystem.
func saveOutbox(path string, state outboxState) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create outbox directory: %w", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode outbox: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".outbox-*.tmp")
	if err != nil {
		return fmt.Errorf("create outbox temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure outbox temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write outbox temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync outbox temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close outbox temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace outbox file: %w", err)
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func initializeOutbox(path string, state outboxState) error {
	if path == "" {
		return nil
	}
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect outbox %q: %w", path, err)
	}
	return saveOutbox(path, state)
}
