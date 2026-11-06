package devicestore

import (
 "testing"
 "fogsense-device-runtime/pkg/device"
)

func TestSnapshotSurvivesReopen(t *testing.T) {
 path:=t.TempDir()+"/device.db"
 s,err:=Open(path); if err!=nil { t.Fatal(err) }
 want:=device.Snapshot{Version:1,DeviceID:"light-1",State:map[string]interface{}{"power":"ON"},Commands:[]device.CommandRecord{{ID:"cmd-1",Status:200,Body:map[string]interface{}{"status":"EXECUTED"}}}}
 if err:=s.Save(want); err!=nil { t.Fatal(err) }; if err:=s.Close(); err!=nil { t.Fatal(err) }
 s,err=Open(path); if err!=nil { t.Fatal(err) }; defer s.Close()
 got,err:=s.Load("light-1"); if err!=nil { t.Fatal(err) }
 if got.State["power"]!="ON" || len(got.Commands)!=1 || got.Commands[0].ID!="cmd-1" { t.Fatalf("unexpected snapshot: %+v",got) }
}
func TestMissingSnapshot(t *testing.T) {
 s,err:=Open(":memory:"); if err!=nil { t.Fatal(err) }; defer s.Close()
 got,err:=s.Load("new-device"); if err!=nil { t.Fatal(err) }
 if got.Version!=1 || got.DeviceID!="new-device" || got.State!=nil { t.Fatalf("unexpected empty snapshot: %+v",got) }
}
