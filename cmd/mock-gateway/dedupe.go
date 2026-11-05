package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func loadSeenMessages(path string) (map[string]bool, error) {
	seen := make(map[string]bool)
	if path == "" {
		return seen, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return seen, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read gateway deduplication store: %w", err)
	}
	if err := json.Unmarshal(data, &seen); err != nil {
		return nil, fmt.Errorf("decode gateway deduplication store: %w", err)
	}
	return seen, nil
}

func saveSeenMessages(path string, seen map[string]bool) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create deduplication directory: %w", err)
	}
	data, err := json.Marshal(seen)
	if err != nil {
		return fmt.Errorf("encode deduplication store: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".dedupe-*.tmp")
	if err != nil {
		return fmt.Errorf("create deduplication temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace deduplication store: %w", err)
	}
	return nil
}

func initializeSeenMessages(path string, seen map[string]bool) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect deduplication store %q: %w", path, err)
	}
	return saveSeenMessages(path, seen)
}
