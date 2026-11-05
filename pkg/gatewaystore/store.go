// Package gatewaystore provides transactional, durable message acceptance for mock gateways.
package gatewaystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrInvalidMessage = errors.New("device ID and message ID are required")

type Store struct {
	db *sql.DB
}

type AcceptedMessage struct {
	DeviceID  string
	MessageID string
	Kind      string
	Payload   json.RawMessage
	AcceptedAt time.Time
}

// Open creates or opens a SQLite database. A single connection keeps connection-local
// pragmas consistent and is sufficient for the mock gateways' serialized handlers.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("gateway store path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("create gateway store directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open gateway SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(err error) (*Store, error) {
		_ = db.Close()
		return nil, err
	}
	for _, pragma := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = FULL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return fail(fmt.Errorf("configure gateway SQLite database: %w", err))
		}
	}
	if path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return fail(fmt.Errorf("enable SQLite WAL: %w", err))
		}
	}
	const schema = `
CREATE TABLE IF NOT EXISTS accepted_messages (
    device_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload BLOB NOT NULL,
    accepted_at TEXT NOT NULL,
    PRIMARY KEY (device_id, message_id)
);
CREATE INDEX IF NOT EXISTS accepted_messages_accepted_at
    ON accepted_messages(accepted_at);
`
	if _, err := db.Exec(schema); err != nil {
		return fail(fmt.Errorf("initialize gateway schema: %w", err))
	}
	return &Store{db: db}, nil
}

// Accept atomically stores the message payload and its deduplication key.
// A duplicate returns duplicate=true without modifying the original payload.
func (s *Store) Accept(ctx context.Context, deviceID, messageID, kind string, payload interface{}) (duplicate bool, err error) {
	if s == nil || s.db == nil {
		return false, errors.New("gateway store is not open")
	}
	if deviceID == "" || messageID == "" || kind == "" {
		return false, ErrInvalidMessage
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("encode accepted message: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin message acceptance transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
INSERT INTO accepted_messages(device_id, message_id, kind, payload, accepted_at)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(device_id, message_id) DO NOTHING
`, deviceID, messageID, kind, encoded, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, fmt.Errorf("persist accepted message: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect message acceptance result: %w", err)
	}
	if rows == 0 {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("finish duplicate message transaction: %w", err)
		}
		return true, nil
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit accepted message: %w", err)
	}
	return false, nil
}

func (s *Store) Get(ctx context.Context, deviceID, messageID string) (AcceptedMessage, error) {
	var message AcceptedMessage
	var payload []byte
	var acceptedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT device_id, message_id, kind, payload, accepted_at
FROM accepted_messages WHERE device_id = ? AND message_id = ?
`, deviceID, messageID).Scan(&message.DeviceID, &message.MessageID, &message.Kind, &payload, &acceptedAt)
	if err != nil {
		return AcceptedMessage{}, err
	}
	message.Payload = append(json.RawMessage(nil), payload...)
	message.AcceptedAt, err = time.Parse(time.RFC3339Nano, acceptedAt)
	if err != nil {
		return AcceptedMessage{}, fmt.Errorf("parse accepted message timestamp: %w", err)
	}
	return message, nil
}

func (s *Store) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accepted_messages").Scan(&count)
	return count, err
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
