package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func loadMQTTSeen(path string) (map[string]bool, error) {
	seen := make(map[string]bool)
	if path == "" {
		return seen, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return seen, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read MQTT gateway deduplication store: %w", err)
	}
	if err := json.Unmarshal(data, &seen); err != nil {
		return nil, fmt.Errorf("decode MQTT gateway deduplication store: %w", err)
	}
	return seen, nil
}

func saveMQTTSeen(path string, seen map[string]bool) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".mqtt-dedupe-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
