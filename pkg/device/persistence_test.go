package device

import (
 "errors"
 "testing"
 "time"
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

func TestRuntimeFaultsSurviveRestart(t *testing.T) {
	store:=&testStateStore{};cfg:=Config{DeviceID:"light-1",Spec:testPersistedSpec(t),Store:store}
	first,err:=NewDeviceWithError(cfg);if err!=nil{t.Fatal(err)}
	for _,command:=range []string{`{"action":"OFFLINE","durationSec":120}`,`{"action":"SPIKE","durationSec":90}`,`{"action":"FLOOD","durationSec":60,"multiplier":5}`}{
		if status,body:=first.ApplyFault([]byte(command));status!=200{t.Fatalf("fault failed: %d %+v",status,body)}
	}
	second,err:=NewDeviceWithError(cfg);if err!=nil{t.Fatal(err)}
	if second.Status!="OFFLINE"||second.OfflineUntil<=second.VT{t.Fatalf("offline not restored: %s %v %v",second.Status,second.OfflineUntil,second.VT)}
	if second.Spike==nil||second.Spike.Until<=second.VT{t.Fatalf("spike not restored: %+v",second.Spike)}
	if second.Flood==nil||second.Flood.Multiplier!=5||second.Flood.Until<=second.VT{t.Fatalf("flood not restored: %+v",second.Flood)}
}
func TestExpiredRuntimeFaultsAreNotRestored(t *testing.T) {
	past:=time.Now().Add(-time.Minute)
	store:=&testStateStore{snapshot:Snapshot{Version:1,DeviceID:"light-1",State:map[string]interface{}{"power":"OFF"},Runtime:&RuntimeSnapshot{DeviceTimeSec:12,Status:"OFFLINE",OfflineExpiresAt:&past,Spike:&PersistedSpikeFault{ExpiresAt:past},Flood:&PersistedFloodFault{Multiplier:5,ExpiresAt:past}}}}
	d,err:=NewDeviceWithError(Config{DeviceID:"light-1",Spec:testPersistedSpec(t),Store:store});if err!=nil{t.Fatal(err)}
	if d.Status!="ONLINE"||d.Spike!=nil||d.Flood!=nil||d.VT!=12{t.Fatalf("expired runtime restored: status=%s spike=%+v flood=%+v vt=%v",d.Status,d.Spike,d.Flood,d.VT)}
}
func TestFaultPersistenceFailureRollsBack(t *testing.T) {
	store:=&testStateStore{fail:true};d,err:=NewDeviceWithError(Config{DeviceID:"light-1",Spec:testPersistedSpec(t),Store:store});if err!=nil{t.Fatal(err)}
	status,body:=d.ApplyFault([]byte(`{"action":"OFFLINE","durationSec":10}`))
	if status!=503||body["reason"]!="PERSISTENCE_FAILED"{t.Fatalf("expected persistence failure: %d %+v",status,body)}
	if d.Status!="ONLINE"{t.Fatalf("fault applied despite persistence failure: %s",d.Status)}
}

func TestPendingCommandAcknowledgementAndStateEventSurviveRestart(t *testing.T) {
	store := &testStateStore{}
	cfg := Config{DeviceID: "light-1", Spec: testPersistedSpec(t), Store: store}

	first, err := NewDeviceWithError(cfg)
	if err != nil {
		t.Fatal(err)
	}
	status, response := first.ExecuteCommand([]byte(`{"commandId":"mqtt-command-1","name":"TURN_ON"}`))
	if status != 200 {
		t.Fatalf("execute command: status=%d response=%+v", status, response)
	}
	messageID, err := first.QueueCommandAck("mqtt-command-1", status, response)
	if err != nil {
		t.Fatalf("queue command acknowledgement: %v", err)
	}

	second, err := NewDeviceWithError(cfg)
	if err != nil {
		t.Fatal(err)
	}
	acks := second.PendingCommandAcks()
	if len(acks) != 1 {
		t.Fatalf("restored command acknowledgements = %d, want 1", len(acks))
	}
	if acks[0].MessageID != messageID || acks[0].CommandID != "mqtt-command-1" {
		t.Fatalf("restored acknowledgement = %+v", acks[0])
	}
	events := second.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("restored events = %d, want 1", len(events))
	}
	if events[0].Event.Type != "STATE_CHANGED" {
		t.Fatalf("restored event type = %q, want STATE_CHANGED", events[0].Event.Type)
	}
	if events[0].Event.Cause["commandId"] != "mqtt-command-1" {
		t.Fatalf("restored event cause = %+v", events[0].Event.Cause)
	}
}

func TestQueueCommandAckRollsBackWhenPersistenceFails(t *testing.T) {
	store := &testStateStore{}
	d, err := NewDeviceWithError(Config{DeviceID: "light-1", Spec: testPersistedSpec(t), Store: store})
	if err != nil {
		t.Fatal(err)
	}
	store.fail = true

	if _, err := d.QueueCommandAck("mqtt-command-2", 200, map[string]interface{}{"status": "EXECUTED"}); err == nil {
		t.Fatal("expected command acknowledgement persistence to fail")
	}
	if pending := d.PendingCommandAcks(); len(pending) != 0 {
		t.Fatalf("pending acknowledgements after failed persistence = %d, want 0", len(pending))
	}
}
