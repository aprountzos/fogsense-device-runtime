package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadExample(t *testing.T, filename string) []byte {
	path := filepath.Join("..", "..", "examples", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read example %s: %v", filename, err)
	}
	return data
}

func TestExamplesValid(t *testing.T) {
	examples := []string{
		"door-lock.yaml",
		"smart-light.yaml",
		"temp-sensor.yaml",
		"thermostat.yaml",
	}

	for _, ex := range examples {
		t.Run(ex, func(t *testing.T) {
			data := loadExample(t, ex)
			res := ValidateSpec(data)
			if !res.OK {
				t.Fatalf("expected example %s to be valid, got errors: %+v", ex, res.Errors)
			}
		})
	}
}

func expectError(t *testing.T, raw []byte, pathPart, msgPart string) {
	res := ValidateSpec(raw)
	if res.OK {
		t.Fatalf("expected spec to be invalid, but validation succeeded")
	}
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e.Path, pathPart) && strings.Contains(e.Message, msgPart) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error at path '%s' containing '%s', got errors: %+v", pathPart, msgPart, res.Errors)
	}
}

func modifyJson(t *testing.T, raw []byte, fn func(m map[string]interface{})) []byte {
	var input interface{}
	if err := yaml.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	input = normalizeYAML(input)
	m := input.(map[string]interface{})
	fn(m)
	res, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDefaultsApplied(t *testing.T) {
	raw := []byte(`{"type":"x1","version":1,"telemetry":[{"name":"a","min":0,"max":1}]}`)
	res := ValidateSpec(raw)
	if !res.OK {
		t.Fatalf("expected valid spec, got errors: %+v", res.Errors)
	}
	if res.Spec.Telemetry[0].IntervalSec != 5 {
		t.Errorf("expected intervalSec=5, got %v", res.Spec.Telemetry[0].IntervalSec)
	}
	if res.Spec.Telemetry[0].Noise != 0 {
		t.Errorf("expected noise=0, got %v", res.Spec.Telemetry[0].Noise)
	}
	if res.Spec.OfflineRate != 0 {
		t.Errorf("expected offlineRate=0, got %v", res.Spec.OfflineRate)
	}
}

func TestStateInitialDefaultsPerType(t *testing.T) {
	raw := []byte(`{
		"type": "x1",
		"version": 1,
		"state": [
			{"name": "e", "type": "enum", "values": ["A", "B"]},
			{"name": "n", "type": "number", "min": 5, "max": 9},
			{"name": "b", "type": "boolean"}
		]
	}`)
	res := ValidateSpec(raw)
	if !res.OK {
		t.Fatalf("expected valid spec, got errors: %+v", res.Errors)
	}
	if res.Spec.State[0].Initial != "A" {
		t.Errorf("expected enum initial='A', got %v", res.Spec.State[0].Initial)
	}
	if res.Spec.State[1].Initial != 5.0 {
		t.Errorf("expected number initial=5, got %v", res.Spec.State[1].Initial)
	}
	if res.Spec.State[2].Initial != false {
		t.Errorf("expected boolean initial=false, got %v", res.Spec.State[2].Initial)
	}
}

func TestValidationErrors(t *testing.T) {
	expectError(t, []byte(`[]`), "", "object")
	expectError(t, []byte(`{"type":"x1","version":1,"bogus":1}`), "bogus", "unknown")
	expectError(t, []byte(`{"type":"Bad Name","version":1,"state":[{"name":"b","type":"boolean"}]}`), "type", "match")
	expectError(t, []byte(`{"type":"x1","version":0,"state":[{"name":"b","type":"boolean"}]}`), "version", "integer")
	expectError(t, []byte(`{"type":"empty","version":1}`), "", "at least one")

	// duplicate metric
	tempData := loadExample(t, "temp-sensor.yaml")
	modTemp := modifyJson(t, tempData, func(m map[string]interface{}) {
		tele := m["telemetry"].([]interface{})
		t1 := tele[1].(map[string]interface{})
		t1["name"] = "temperature"
	})
	expectError(t, modTemp, "telemetry[1].name", "duplicate")

	// min >= max
	modMinMax := modifyJson(t, tempData, func(m map[string]interface{}) {
		tele := m["telemetry"].([]interface{})
		t0 := tele[0].(map[string]interface{})
		t0["min"] = 40.0
	})
	expectError(t, modMinMax, "telemetry[0]", "min must be less")

	// out of range rates
	modRates := modifyJson(t, tempData, func(m map[string]interface{}) {
		tele := m["telemetry"].([]interface{})
		t0 := tele[0].(map[string]interface{})
		t0["anomalyRate"] = 2.0
		t1 := tele[1].(map[string]interface{})
		t1["intervalSec"] = 0.0
	})
	expectError(t, modRates, "anomalyRate", "between 0 and 1")
	expectError(t, modRates, "intervalSec", "between 1 and 3600")

	// too many metrics
	rawTooMany := []byte(`{
		"type": "big", "version": 1,
		"telemetry": [
			{"name":"m0","min":0,"max":1},{"name":"m1","min":0,"max":1},{"name":"m2","min":0,"max":1},
			{"name":"m3","min":0,"max":1},{"name":"m4","min":0,"max":1},{"name":"m5","min":0,"max":1},
			{"name":"m6","min":0,"max":1},{"name":"m7","min":0,"max":1},{"name":"m8","min":0,"max":1}
		]
	}`)
	expectError(t, rawTooMany, "telemetry", "at most 8")

	// enum without values and enum with $
	expectError(t, []byte(`{"type":"x1","version":1,"state":[{"name":"e","type":"enum"}]}`), "values", "enum requires")
	expectError(t, []byte(`{"type":"x1","version":1,"state":[{"name":"e","type":"enum","values":["$A"]}]}`), "values[0]", "'$'")

	// initial not fitting
	expectError(t, []byte(`{"type":"x1","version":1,"state":[{"name":"n","type":"number","min":0,"max":1,"initial":5}]}`), "initial", "does not fit")

	// effect on unknown state field
	smartData := loadExample(t, "smart-light.yaml")
	modUnknownEffect := modifyJson(t, smartData, func(m map[string]interface{}) {
		cmds := m["commands"].([]interface{})
		c0 := cmds[0].(map[string]interface{})
		c0["effects"] = map[string]interface{}{"nope": "ON"}
	})
	expectError(t, modUnknownEffect, "effects.nope", "unknown state field")

	// constant not fitting
	modBadConstant := modifyJson(t, smartData, func(m map[string]interface{}) {
		cmds := m["commands"].([]interface{})
		c0 := cmds[0].(map[string]interface{})
		c0["effects"] = map[string]interface{}{"power": "MAYBE"}
	})
	expectError(t, modBadConstant, "effects.power", "does not fit")

	// reference to unknown param
	modUnknownParam := modifyJson(t, smartData, func(m map[string]interface{}) {
		cmds := m["commands"].([]interface{})
		c2 := cmds[2].(map[string]interface{})
		c2["effects"] = map[string]interface{}{"brightness": "$nothing"}
	})
	expectError(t, modUnknownParam, "effects.brightness", "unknown param")

	// param/state type mismatch
	modMismatch := modifyJson(t, smartData, func(m map[string]interface{}) {
		cmds := m["commands"].([]interface{})
		c2 := cmds[2].(map[string]interface{})
		c2Params := c2["params"].(map[string]interface{})
		c2Params["level"] = map[string]interface{}{"type": "enum", "values": []interface{}{"ON"}}
	})
	expectError(t, modMismatch, "effects.brightness", "type differs")

	// param range outside state range
	modRangeOutside := modifyJson(t, smartData, func(m map[string]interface{}) {
		cmds := m["commands"].([]interface{})
		c2 := cmds[2].(map[string]interface{})
		c2Params := c2["params"].(map[string]interface{})
		c2Params["level"] = map[string]interface{}{"type": "number", "min": 0.0, "max": 500.0}
	})
	expectError(t, modRangeOutside, "effects.brightness", "range must lie within")

	// follows pointing to non-numeric state
	thermoData := loadExample(t, "thermostat.yaml")
	modFollowsNonNum := modifyJson(t, thermoData, func(m map[string]interface{}) {
		tele := m["telemetry"].([]interface{})
		t0 := tele[0].(map[string]interface{})
		f := t0["follows"].(map[string]interface{})
		f["state"] = "mode"
	})
	expectError(t, modFollowsNonNum, "follows.state", "numeric state field")

	// follows.when with unknown field or bad value
	modFollowsWhen := modifyJson(t, thermoData, func(m map[string]interface{}) {
		tele := m["telemetry"].([]interface{})
		t0 := tele[0].(map[string]interface{})
		f := t0["follows"].(map[string]interface{})
		f["when"] = map[string]interface{}{"ghost": 1}
	})
	expectError(t, modFollowsWhen, "follows.when.ghost", "unknown state field")

	// non-boolean userControllable
	modUC := modifyJson(t, smartData, func(m map[string]interface{}) {
		cmds := m["commands"].([]interface{})
		c0 := cmds[0].(map[string]interface{})
		c0["userControllable"] = "yes"
	})
	expectError(t, modUC, "userControllable", "boolean")
}
