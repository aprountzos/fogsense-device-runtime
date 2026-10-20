package device

import (
	"math"
	"testing"

	"fogsense-device-runtime/pkg/spec"
)

func makeTestDevice(specJson string, seed int64, timeScale float64) *Device {
	res := spec.ValidateSpec([]byte(specJson))
	if !res.OK {
		panic("invalid test spec: " + res.Errors[0].Message)
	}
	s := seed
	return NewDevice(Config{
		DeviceID:  "test-1",
		Spec:      res.Spec,
		Seed:      &s,
		TimeScale: timeScale,
	})
}

func TestDeviceTelemetryInterval(t *testing.T) {
	specRaw := `{
		"type": "sensor", "version": 1,
		"telemetry": [
			{"name": "fast", "min": 0, "max": 10, "intervalSec": 1},
			{"name": "slow", "min": 0, "max": 10, "intervalSec": 5}
		]
	}`

	d := makeTestDevice(specRaw, 42, 1)

	// First tick at 0 seconds: both metrics are due
	r0 := d.Tick(0)
	if len(r0) != 2 {
		t.Fatalf("expected 2 readings initially, got %d", len(r0))
	}

	// 1 second later: only 'fast' should emit
	r1 := d.Tick(1.0)
	if len(r1) != 1 || r1[0].Metric != "fast" {
		t.Fatalf("expected 1 reading for 'fast', got %+v", r1)
	}
}

func TestDeviceAnomaly(t *testing.T) {
	specRaw := `{
		"type": "sensor", "version": 1,
		"telemetry": [{"name": "m", "min": 10, "max": 20, "anomalyRate": 1.0, "intervalSec": 1}]
	}`
	d := makeTestDevice(specRaw, 42, 1)
	r := d.Tick(0)
	if len(r) != 1 {
		t.Fatalf("expected 1 reading, got %d", len(r))
	}
	if r[0].Value >= 10 && r[0].Value <= 20 {
		t.Fatalf("anomalyRate=1 should produce out of range value, got %v", r[0].Value)
	}
}

func TestDeviceCommandsAndIdempotency(t *testing.T) {
	specRaw := `{
		"type": "light", "version": 1,
		"state": [
			{"name": "power", "type": "enum", "values": ["ON", "OFF"], "initial": "OFF"},
			{"name": "brightness", "type": "number", "min": 0, "max": 100, "initial": 0}
		],
		"commands": [
			{"name": "TURN_ON", "effects": {"power": "ON"}},
			{
				"name": "SET_LEVEL",
				"params": {"level": {"type": "number", "min": 0, "max": 100}},
				"effects": {"brightness": "$level", "power": "ON"}
			}
		]
	}`

	d := makeTestDevice(specRaw, 42, 1)

	var events []Event
	d.OnEvent(func(e Event) {
		events = append(events, e)
	})

	// Execute SET_LEVEL
	req1 := `{"commandId":"c1","name":"SET_LEVEL","params":{"level":75}}`
	status, body := d.ExecuteCommand([]byte(req1))
	if status != 200 {
		t.Fatalf("expected status 200, got %d: %+v", status, body)
	}
	if body["status"] != "EXECUTED" || body["changed"] != true {
		t.Fatalf("expected status=EXECUTED and changed=true, got %+v", body)
	}
	st := body["state"].(map[string]interface{})
	if st["brightness"] != 75.0 || st["power"] != "ON" {
		t.Fatalf("expected brightness=75 and power=ON, got %+v", st)
	}
	if len(events) != 1 || events[0].Type != "STATE_CHANGED" {
		t.Fatalf("expected 1 STATE_CHANGED event, got %+v", events)
	}

	// Idempotent duplicate replay
	statusDup, bodyDup := d.ExecuteCommand([]byte(req1))
	if statusDup != 200 {
		t.Fatalf("expected status 200 on duplicate, got %d", statusDup)
	}
	if bodyDup["duplicate"] != true {
		t.Fatalf("expected duplicate=true, got %+v", bodyDup)
	}
	// No new event should have been emitted
	if len(events) != 1 {
		t.Fatalf("duplicate command emitted extra events: %+v", events)
	}

	// Invalid param rejected
	badReq := `{"commandId":"c2","name":"SET_LEVEL","params":{"level":150}}`
	statusBad, bodyBad := d.ExecuteCommand([]byte(badReq))
	if statusBad != 400 || bodyBad["reason"] != "INVALID_PARAMS" {
		t.Fatalf("expected 400 INVALID_PARAMS, got %d: %+v", statusBad, bodyBad)
	}
}

func TestDeviceOfflineRejectionsNotCached(t *testing.T) {
	specRaw := `{
		"type": "light", "version": 1,
		"state": [{"name": "on", "type": "boolean", "initial": false}],
		"commands": [{"name": "TOGGLE", "effects": {"on": true}}]
	}`
	d := makeTestDevice(specRaw, 42, 1)

	// Inject offline fault
	faultRaw := `{"action":"OFFLINE","durationSec":10}`
	fStatus, _ := d.ApplyFault([]byte(faultRaw))
	if fStatus != 200 || d.Status != "OFFLINE" {
		t.Fatalf("expected device OFFLINE, got %s", d.Status)
	}

	// Sending command while offline yields 503
	cmd := `{"commandId":"c-off","name":"TOGGLE"}`
	s1, b1 := d.ExecuteCommand([]byte(cmd))
	if s1 != 503 || b1["reason"] != "DEVICE_OFFLINE" {
		t.Fatalf("expected 503 DEVICE_OFFLINE, got %d: %+v", s1, b1)
	}

	// Clear fault
	d.ApplyFault([]byte(`{"action":"CLEAR"}`))
	if d.Status != "ONLINE" {
		t.Fatalf("expected device ONLINE after CLEAR, got %s", d.Status)
	}

	// The same command ID MUST succeed on retry (rejection not cached)
	s2, b2 := d.ExecuteCommand([]byte(cmd))
	if s2 != 200 || b2["status"] != "EXECUTED" {
		t.Fatalf("expected retry after recovery to succeed with 200, got %d: %+v", s2, b2)
	}
	if b2["duplicate"] == true {
		t.Fatalf("retry should not be marked duplicate")
	}
}

func TestDeviceStateFollows(t *testing.T) {
	specRaw := `{
		"type": "thermo", "version": 1,
		"state": [
			{"name": "power", "type": "enum", "values": ["ON", "OFF"], "initial": "ON"},
			{"name": "target", "type": "number", "min": 10, "max": 30, "initial": 25}
		],
		"telemetry": [
			{
				"name": "temp", "min": 10, "max": 30, "noise": 0, "intervalSec": 1,
				"follows": {"state": "target", "when": {"power": "ON"}, "rate": 0.5}
			}
		]
	}`

	d := makeTestDevice(specRaw, 42, 1)
	// Initial x is (10+30)/2 = 20. Target is 25.
	// Tick 1: target=25, x moves: 20 + (25-20)*0.5 = 22.5
	r1 := d.Tick(1.0)
	if len(r1) != 1 {
		t.Fatalf("expected 1 reading, got %d", len(r1))
	}
	if math.Abs(r1[0].Value-22.5) > 0.05 {
		t.Fatalf("expected value ~22.5, got %v", r1[0].Value)
	}
}

func TestDeviceInfoAndStateView(t *testing.T) {
	specRaw := `{
		"type": "sensor", "version": 1, "description": "a test sensor",
		"telemetry": [{"name": "t", "min": 0, "max": 100, "intervalSec": 10}],
		"state": [{"name": "s", "type": "boolean", "initial": true}],
		"commands": [{"name": "C", "userControllable": true}]
	}`
	d := makeTestDevice(specRaw, 42, 1)

	info := d.Info()
	if info["type"] != "sensor" || info["description"] != "a test sensor" {
		t.Fatalf("unexpected info: %+v", info)
	}

	state := d.StateView()
	stMap := state["state"].(map[string]interface{})
	if stMap["s"] != true {
		t.Fatalf("unexpected state: %+v", state)
	}
}
