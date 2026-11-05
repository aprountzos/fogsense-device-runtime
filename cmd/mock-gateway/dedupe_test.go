package main

import (
	"path/filepath"
	"testing"
)

func TestSeenMessagesSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dedupe", "messages.json")
	seen := map[string]bool{"light-1:message-1": true}
	if err := saveSeenMessages(path, seen); err != nil {
		t.Fatalf("save dedupe store: %v", err)
	}
	recovered, err := loadSeenMessages(path)
	if err != nil {
		t.Fatalf("load dedupe store: %v", err)
	}
	if !recovered["light-1:message-1"] {
		t.Fatal("expected message ID to survive gateway restart")
	}
}
