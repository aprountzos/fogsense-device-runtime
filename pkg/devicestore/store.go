package devicestore

import (
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"

 "fogsense-device-runtime/pkg/device"
 _ "modernc.org/sqlite"
)

type Store struct { db *sql.DB }

func Open(path string) (*Store,error) {
 if path=="" { return nil,errors.New("device state path is required") }
 if path!=":memory:" { if err:=os.MkdirAll(filepath.Dir(path),0700); err!=nil { return nil,fmt.Errorf("create device state directory: %w",err) } }
 db,err:=sql.Open("sqlite",path); if err!=nil { return nil,fmt.Errorf("open device state database: %w",err) }
 db.SetMaxOpenConns(1); db.SetMaxIdleConns(1)
 fail:=func(err error)(*Store,error){ _=db.Close(); return nil,err }
 for _,pragma:=range []string{"PRAGMA busy_timeout = 5000","PRAGMA synchronous = FULL"} { if _,err:=db.Exec(pragma); err!=nil { return fail(fmt.Errorf("configure device state database: %w",err)) } }
 if path!=":memory:" { if _,err:=db.Exec("PRAGMA journal_mode = WAL"); err!=nil { return fail(fmt.Errorf("enable device state WAL: %w",err)) } }
 _,err=db.Exec(`CREATE TABLE IF NOT EXISTS device_snapshots (
 device_id TEXT PRIMARY KEY, version INTEGER NOT NULL, snapshot BLOB NOT NULL,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
)`)
 if err!=nil { return fail(fmt.Errorf("initialize device state schema: %w",err)) }
 return &Store{db:db},nil
}

func (s *Store) Load(deviceID string)(device.Snapshot,error) {
 if s==nil || s.db==nil { return device.Snapshot{},errors.New("device state store is not open") }
 var raw []byte
 err:=s.db.QueryRow("SELECT snapshot FROM device_snapshots WHERE device_id = ?",deviceID).Scan(&raw)
 if errors.Is(err,sql.ErrNoRows) { return device.Snapshot{Version:1,DeviceID:deviceID},nil }
 if err!=nil { return device.Snapshot{},fmt.Errorf("read device snapshot: %w",err) }
 var snapshot device.Snapshot
 if err:=json.Unmarshal(raw,&snapshot); err!=nil { return device.Snapshot{},fmt.Errorf("decode device snapshot (database may be corrupt): %w",err) }
 return snapshot,nil
}

func (s *Store) Save(snapshot device.Snapshot) error {
 if s==nil || s.db==nil { return errors.New("device state store is not open") }
 if snapshot.Version!=1 || snapshot.DeviceID=="" || snapshot.State==nil { return errors.New("refusing to save invalid device snapshot") }
 raw,err:=json.Marshal(snapshot); if err!=nil { return fmt.Errorf("encode device snapshot: %w",err) }
 tx,err:=s.db.Begin(); if err!=nil { return fmt.Errorf("begin device state transaction: %w",err) }
 defer func(){ _=tx.Rollback() }()
 _,err=tx.Exec(`INSERT INTO device_snapshots(device_id,version,snapshot,updated_at)
 VALUES(?,?,?,CURRENT_TIMESTAMP) ON CONFLICT(device_id) DO UPDATE SET
 version=excluded.version,snapshot=excluded.snapshot,updated_at=CURRENT_TIMESTAMP`,
 snapshot.DeviceID,snapshot.Version,raw)
 if err!=nil { return fmt.Errorf("write device snapshot: %w",err) }
 if err:=tx.Commit(); err!=nil { return fmt.Errorf("commit device snapshot: %w",err) }
 return nil
}

func (s *Store) Close() error { if s==nil || s.db==nil { return nil }; return s.db.Close() }
