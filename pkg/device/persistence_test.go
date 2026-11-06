package device

import (
 "errors"
 "testing"
 "fogsense-device-runtime/pkg/spec"
)

type testStateStore struct { snapshot Snapshot; fail bool }
func (s *testStateStore) Load(id string)(Snapshot,error) {
 if s.snapshot.DeviceID=="" { return Snapshot{Version:1,DeviceID:id},nil }; return s.snapshot,nil
}
func (s *testStateStore) Save(v Snapshot)error { if s.fail{return errors.New("disk full")}; s.snapshot=v; return nil }

func testPersistedSpec(t *testing.T)*spec.DeviceSpec {
 raw:=`{"type":"light","version":1,"state":[{"name":"power","type":"enum","values":["ON","OFF"],"initial":"OFF"}],"commands":[{"name":"TURN_ON","effects":{"power":"ON"}}]}`
 v:=spec.ValidateSpec([]byte(raw)); if !v.OK { t.Fatalf("invalid test spec: %+v",v.Errors) }; return v.Spec
}
func TestCommandIdempotencySurvivesRestart(t *testing.T) {
 store:=&testStateStore{}; cfg:=Config{DeviceID:"light-1",Spec:testPersistedSpec(t),Store:store}
 first,err:=NewDeviceWithError(cfg); if err!=nil { t.Fatal(err) }
 cmd:=[]byte(`{"commandId":"persisted-1","name":"TURN_ON"}`)
 status,body:=first.ExecuteCommand(cmd); if status!=200 || body["status"]!="EXECUTED" { t.Fatalf("first command: %d %+v",status,body) }
 second,err:=NewDeviceWithError(cfg); if err!=nil { t.Fatal(err) }
 status,body=second.ExecuteCommand(cmd)
 if status!=200 || body["duplicate"]!=true { t.Fatalf("expected persisted duplicate: %d %+v",status,body) }
 if second.State["power"]!="ON" { t.Fatalf("state not restored: %+v",second.State) }
}
func TestPersistenceFailureDoesNotApplyCommand(t *testing.T) {
 store:=&testStateStore{fail:true}; d,err:=NewDeviceWithError(Config{DeviceID:"light-1",Spec:testPersistedSpec(t),Store:store}); if err!=nil { t.Fatal(err) }
 status,body:=d.ExecuteCommand([]byte(`{"commandId":"fail-1","name":"TURN_ON"}`))
 if status!=503 || body["reason"]!="PERSISTENCE_FAILED" { t.Fatalf("expected 503: %d %+v",status,body) }
 if d.State["power"]!="OFF" { t.Fatalf("state changed despite persistence failure: %+v",d.State) }
 if _,ok:=d.Recent["fail-1"]; ok { t.Fatal("failed command was cached") }
}
func TestUnsupportedSnapshotVersionFailsStartup(t *testing.T) {
 store:=&testStateStore{snapshot:Snapshot{Version:2,DeviceID:"light-1",State:map[string]interface{}{"power":"ON"}}}
 _,err:=NewDeviceWithError(Config{DeviceID:"light-1",Spec:testPersistedSpec(t),Store:store})
 if err==nil { t.Fatal("expected unsupported snapshot version to fail startup") }
}
