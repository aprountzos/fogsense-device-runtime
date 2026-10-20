package device

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"fogsense-device-runtime/pkg/prng"
	"fogsense-device-runtime/pkg/spec"
)

const MaxRecent = 1000

func round2(n float64) float64 {
	return math.Round(n*100) / 100
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type Reading struct {
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	TS     string  `json:"ts"`
}

type MetricRuntime struct {
	Def    spec.MetricSpec
	X      float64
	NextAt float64
	Last   *Reading
}

type SpikeFault struct {
	Metric *string `json:"metric"`
	Until  float64 `json:"until"`
}

type FloodFault struct {
	Multiplier float64 `json:"multiplier"`
	Until      float64 `json:"until"`
}

type Event struct {
	Type   string                 `json:"type"`
	Status string                 `json:"status,omitempty"`
	Reason string                 `json:"reason,omitempty"`
	State  map[string]interface{} `json:"state,omitempty"`
	Cause  map[string]interface{} `json:"cause,omitempty"`
}

type CachedCommand struct {
	Status int
	Body   map[string]interface{}
}

type Device struct {
	mu           sync.Mutex
	DeviceID     string
	Spec         *spec.DeviceSpec
	TimeScale    float64
	Rng          *prng.Mulberry32
	BootID       string
	StartedAt    time.Time
	VT           float64
	Status       string
	OfflineUntil float64
	Spike        *SpikeFault
	Flood        *FloodFault
	State        map[string]interface{}
	Metrics      map[string]*MetricRuntime
	MetricOrder  []string
	Recent       map[string]CachedCommand
	RecentOrder  []string
	Listeners    []func(Event)
}

type Config struct {
	DeviceID  string
	Spec      *spec.DeviceSpec
	Seed      *int64
	TimeScale float64
}

func NewDevice(cfg Config) *Device {
	var seed uint32
	if cfg.Seed != nil {
		seed = uint32(*cfg.Seed)
	} else {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		seed = (uint32(b[0]) << 24) | (uint32(b[1]) << 16) | (uint32(b[2]) << 8) | uint32(b[3])
		if seed == 0 {
			seed = 12345
		}
	}

	ts := cfg.TimeScale
	if ts <= 0 {
		ts = 1.0
	}

	d := &Device{
		DeviceID:    cfg.DeviceID,
		Spec:        cfg.Spec,
		TimeScale:   ts,
		Rng:         prng.NewMulberry32(seed),
		BootID:      generateUUID(),
		StartedAt:   time.Now(),
		VT:          0,
		Status:      "ONLINE",
		State:       make(map[string]interface{}),
		Metrics:     make(map[string]*MetricRuntime),
		Recent:      make(map[string]CachedCommand),
		RecentOrder: make([]string, 0, MaxRecent),
	}

	for _, s := range cfg.Spec.State {
		d.State[s.Name] = s.Initial
	}

	for _, m := range cfg.Spec.Telemetry {
		d.MetricOrder = append(d.MetricOrder, m.Name)
		d.Metrics[m.Name] = &MetricRuntime{
			Def:    m,
			X:      (m.Min + m.Max) / 2.0,
			NextAt: 0,
			Last:   nil,
		}
	}

	return d
}

func (d *Device) OnEvent(fn func(Event)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Listeners = append(d.Listeners, fn)
}

func (d *Device) emit(evt Event) {
	for _, fn := range d.Listeners {
		fn(evt)
	}
}

func (d *Device) goOffline(durationSec float64, reason string) {
	d.Status = "OFFLINE"
	d.OfflineUntil = d.VT + durationSec
	d.emit(Event{Type: "STATUS_CHANGED", Status: "OFFLINE", Reason: reason})
}

func (d *Device) goOnline(reason string) {
	d.Status = "ONLINE"
	d.OfflineUntil = 0
	for _, m := range d.Metrics {
		m.NextAt = d.VT
	}
	d.emit(Event{Type: "STATUS_CHANGED", Status: "ONLINE", Reason: reason})
}

func (d *Device) followTarget(def spec.MetricSpec) *float64 {
	if def.Follows == nil {
		return nil
	}
	for k, v := range def.Follows.When {
		if d.State[k] != v {
			return nil
		}
	}
	targetVal, exists := d.State[def.Follows.State]
	if !exists {
		return nil
	}
	switch num := targetVal.(type) {
	case float64:
		return &num
	case int:
		f := float64(num)
		return &f
	default:
		return nil
	}
}

func (d *Device) sample(m *MetricRuntime) Reading {
	def := m.Def
	rangeVal := def.Max - def.Min
	spikeActive := d.Spike != nil && d.VT < d.Spike.Until && (d.Spike.Metric == nil || *d.Spike.Metric == def.Name)

	var value float64
	if spikeActive || d.Rng.Next() < def.AnomalyRate {
		above := d.Rng.Next() < 0.5
		margin := (0.1 + d.Rng.Next()*0.4) * rangeVal
		if above {
			value = def.Max + margin
		} else {
			value = def.Min - margin
		}
	} else {
		target := d.followTarget(def)
		if target != nil {
			m.X += (*target - m.X) * def.Follows.Rate
		} else {
			if def.Follows != nil {
				m.X += ((def.Min+def.Max)/2.0 - m.X) * 0.1
			}
			m.X += (d.Rng.Next() - 0.5) * 0.1 * rangeVal
			if def.Follows == nil {
				m.X = math.Min(def.Max, math.Max(def.Min, m.X))
			}
		}
		value = m.X + d.Rng.NextGaussian()*def.Noise
	}

	value = round2(value)
	nowIso := time.Now().UTC().Format(time.RFC3339Nano)
	reading := Reading{
		Metric: def.Name,
		Value:  value,
		Unit:   def.Unit,
		TS:     nowIso,
	}
	m.Last = &reading
	return reading
}

func (d *Device) Tick(dtRealSec float64) []Reading {
	d.mu.Lock()
	defer d.mu.Unlock()

	dt := dtRealSec * d.TimeScale
	d.VT += dt

	if d.Spike != nil && d.VT >= d.Spike.Until {
		d.Spike = nil
	}
	if d.Flood != nil && d.VT >= d.Flood.Until {
		d.Flood = nil
	}

	if d.Status == "OFFLINE" {
		if d.VT >= d.OfflineUntil {
			d.goOnline("RECOVERED")
		}
		return nil
	}

	if d.Spec.OfflineRate > 0 {
		p := 1.0 - math.Pow(1.0-d.Spec.OfflineRate, dt)
		if d.Rng.Next() < p {
			d.goOffline(5.0+d.Rng.Next()*15.0, "RANDOM")
			return nil
		}
	}

	mult := 1.0
	if d.Flood != nil {
		mult = d.Flood.Multiplier
	}

	var readings []Reading
	for _, name := range d.MetricOrder {
		m := d.Metrics[name]
		if d.VT >= m.NextAt {
			readings = append(readings, d.sample(m))
			m.NextAt = d.VT + m.Def.IntervalSec/mult
		}
	}

	return readings
}

func (d *Device) remember(commandId string, status int, body map[string]interface{}) (int, map[string]interface{}) {
	if len(d.RecentOrder) >= MaxRecent {
		oldest := d.RecentOrder[0]
		d.RecentOrder = d.RecentOrder[1:]
		delete(d.Recent, oldest)
	}
	d.Recent[commandId] = CachedCommand{Status: status, Body: body}
	d.RecentOrder = append(d.RecentOrder, commandId)
	return status, body
}

func (d *Device) ExecuteCommand(raw []byte) (int, map[string]interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var req map[string]interface{}
	if err := json.Unmarshal(raw, &req); err != nil {
		return 400, map[string]interface{}{
			"status": "REJECTED",
			"reason": "INVALID_REQUEST",
			"message": "commandId (string, 1-100 chars) and name (string) are required",
		}
	}

	cmdID, hasCmdID := req["commandId"].(string)
	name, hasName := req["name"].(string)

	reject := func(status int, reason string, extra map[string]interface{}) (int, map[string]interface{}) {
		body := map[string]interface{}{
			"status": "REJECTED",
			"reason": reason,
		}
		if hasCmdID {
			body["commandId"] = cmdID
		}
		for k, v := range extra {
			body[k] = v
		}
		return status, body
	}

	rememberReject := func(status int, reason string, extra map[string]interface{}) (int, map[string]interface{}) {
		st, body := reject(status, reason, extra)
		return d.remember(cmdID, st, body)
	}

	if !hasCmdID || len(cmdID) < 1 || len(cmdID) > 100 || !hasName {
		return reject(400, "INVALID_REQUEST", map[string]interface{}{
			"message": "commandId (string, 1-100 chars) and name (string) are required",
		})
	}

	if cached, ok := d.Recent[cmdID]; ok {
		dupBody := make(map[string]interface{})
		for k, v := range cached.Body {
			dupBody[k] = v
		}
		dupBody["duplicate"] = true
		return cached.Status, dupBody
	}

	if d.Status != "ONLINE" {
		return reject(503, "DEVICE_OFFLINE", nil)
	}

	var targetCmd *spec.CommandSpec
	for _, c := range d.Spec.Commands {
		if c.Name == name {
			cmdCopy := c
			targetCmd = &cmdCopy
			break
		}
	}

	if targetCmd == nil {
		return rememberReject(400, "UNKNOWN_COMMAND", nil)
	}

	params := make(map[string]interface{})
	if pRaw, exists := req["params"]; exists {
		pMap, ok := pRaw.(map[string]interface{})
		if !ok {
			return rememberReject(400, "INVALID_PARAMS", map[string]interface{}{"message": "params must be an object"})
		}
		params = pMap
	}

	for k := range params {
		if _, exists := targetCmd.Params[k]; !exists {
			return rememberReject(400, "INVALID_PARAMS", map[string]interface{}{"message": fmt.Sprintf("unknown param '%s'", k)})
		}
	}

	for pn, pDef := range targetCmd.Params {
		val, exists := params[pn]
		if !exists {
			return rememberReject(400, "INVALID_PARAMS", map[string]interface{}{"message": fmt.Sprintf("missing param '%s'", pn)})
		}
		if !spec.ValueFits(pDef, val) {
			return rememberReject(400, "INVALID_PARAMS", map[string]interface{}{"message": fmt.Sprintf("invalid value for param '%s'", pn)})
		}
	}

	beforeBytes, _ := json.Marshal(d.State)
	for field, val := range targetCmd.Effects {
		if strVal, isStr := val.(string); isStr && len(strVal) > 1 && strVal[0] == '$' {
			d.State[field] = params[strVal[1:]]
		} else {
			d.State[field] = val
		}
	}
	afterBytes, _ := json.Marshal(d.State)
	changed := string(beforeBytes) != string(afterBytes)

	stateCopy := make(map[string]interface{})
	for k, v := range d.State {
		stateCopy[k] = v
	}

	resBody := map[string]interface{}{
		"commandId": cmdID,
		"status":    "EXECUTED",
		"state":     stateCopy,
		"changed":   changed,
	}

	status, savedBody := d.remember(cmdID, 200, resBody)
	if changed {
		d.emit(Event{
			Type:  "STATE_CHANGED",
			State: stateCopy,
			Cause: map[string]interface{}{"commandId": cmdID},
		})
	}

	return status, savedBody
}

func (d *Device) ApplyFault(raw []byte) (int, map[string]interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var req map[string]interface{}
	if err := json.Unmarshal(raw, &req); err != nil {
		return 400, map[string]interface{}{"error": "action is required (OFFLINE | SPIKE | FLOOD | CLEAR)"}
	}

	action, ok := req["action"].(string)
	if !ok {
		return 400, map[string]interface{}{"error": "action is required (OFFLINE | SPIKE | FLOOD | CLEAR)"}
	}

	var dur *float64
	if dVal, exists := req["durationSec"]; exists {
		var durFloat float64
		switch n := dVal.(type) {
		case float64:
			durFloat = n
		case int:
			durFloat = float64(n)
		default:
			return 400, map[string]interface{}{"error": "durationSec must be a number between 0 and 86400"}
		}
		if durFloat <= 0 || durFloat > 86400 {
			return 400, map[string]interface{}{"error": "durationSec must be a number between 0 and 86400"}
		}
		dur = &durFloat
	}

	durationOrDefault := func(def float64) float64 {
		if dur != nil {
			return *dur
		}
		return def
	}

	switch action {
	case "OFFLINE":
		dSec := durationOrDefault(60.0)
		if d.Status == "ONLINE" {
			d.goOffline(dSec, "FAULT_INJECTED")
		} else {
			d.OfflineUntil = d.VT + dSec
		}
		return 200, map[string]interface{}{"ok": true, "action": "OFFLINE"}

	case "SPIKE":
		var metricPtr *string
		if mVal, exists := req["metric"]; exists {
			mStr, ok := mVal.(string)
			if !ok {
				return 400, map[string]interface{}{"error": "metric must be a string"}
			}
			if _, exists := d.Metrics[mStr]; !exists {
				return 400, map[string]interface{}{"error": fmt.Sprintf("unknown metric '%s'", mStr)}
			}
			metricPtr = &mStr
		}
		dSec := durationOrDefault(30.0)
		d.Spike = &SpikeFault{Metric: metricPtr, Until: d.VT + dSec}
		res := map[string]interface{}{"ok": true, "action": "SPIKE"}
		if metricPtr != nil {
			res["metric"] = *metricPtr
		}
		return 200, res

	case "FLOOD":
		mult := 10.0
		if mVal, exists := req["multiplier"]; exists {
			switch n := mVal.(type) {
			case float64:
				mult = n
			case int:
				mult = float64(n)
			default:
				return 400, map[string]interface{}{"error": "multiplier must be between 2 and 100"}
			}
			if mult < 2 || mult > 100 {
				return 400, map[string]interface{}{"error": "multiplier must be between 2 and 100"}
			}
		}
		dSec := durationOrDefault(30.0)
		d.Flood = &FloodFault{Multiplier: mult, Until: d.VT + dSec}
		return 200, map[string]interface{}{"ok": true, "action": "FLOOD", "multiplier": mult}

	case "CLEAR":
		d.Spike = nil
		d.Flood = nil
		if d.Status == "OFFLINE" {
			d.goOnline("FAULT_CLEARED")
		}
		return 200, map[string]interface{}{"ok": true, "action": "CLEAR"}

	default:
		return 400, map[string]interface{}{"error": fmt.Sprintf("unknown action '%s'", action)}
	}
}

func (d *Device) ControlStatus() map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()

	var offUntil interface{} = nil
	if d.Status == "OFFLINE" {
		offUntil = round2(d.OfflineUntil)
	}

	return map[string]interface{}{
		"deviceId":      d.DeviceID,
		"status":        d.Status,
		"deviceTimeSec": round2(d.VT),
		"offlineUntil":  offUntil,
		"spike":         d.Spike,
		"flood":         d.Flood,
	}
}

func (d *Device) Info() map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()

	tele := make([]map[string]interface{}, 0, len(d.Spec.Telemetry))
	for _, m := range d.Spec.Telemetry {
		tele = append(tele, map[string]interface{}{
			"name":        m.Name,
			"unit":        m.Unit,
			"min":         m.Min,
			"max":         m.Max,
			"intervalSec": m.IntervalSec,
		})
	}

	cmds := make([]map[string]interface{}, 0, len(d.Spec.Commands))
	for _, c := range d.Spec.Commands {
		cmds = append(cmds, map[string]interface{}{
			"name":             c.Name,
			"params":           c.Params,
			"userControllable": c.UserControllable,
		})
	}

	stateFields := make([]spec.StateFieldSpec, 0, len(d.Spec.State))
	if d.Spec.State != nil {
		stateFields = append(stateFields, d.Spec.State...)
	}

	return map[string]interface{}{
		"deviceId":       d.DeviceID,
		"bootId":         d.BootID,
		"runtimeVersion": "1.0.0",
		"type":           d.Spec.Type,
		"version":        d.Spec.Version,
		"description":    d.Spec.Description,
		"status":         d.Status,
		"telemetry":      tele,
		"state":          stateFields,
		"commands":       cmds,
	}
}

func (d *Device) StateView() map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()

	stateCopy := make(map[string]interface{})
	for k, v := range d.State {
		stateCopy[k] = v
	}

	lastTele := make(map[string]interface{})
	for name, m := range d.Metrics {
		if m.Last != nil {
			lastTele[name] = map[string]interface{}{
				"value": m.Last.Value,
				"ts":    m.Last.TS,
			}
		}
	}

	uptime := int(time.Since(d.StartedAt).Seconds())

	return map[string]interface{}{
		"deviceId":      d.DeviceID,
		"bootId":        d.BootID,
		"status":        d.Status,
		"state":         stateCopy,
		"lastTelemetry": lastTele,
		"uptimeSec":     uptime,
	}
}
