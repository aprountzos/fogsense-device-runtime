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
const MaxPendingTelemetry = 500
const MaxPendingCommandAcks = 1000

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
	Status int `json:"status"`
	Body map[string]interface{} `json:"body"`
}
type CommandRecord struct {
	ID string `json:"id"`
	Status int `json:"status"`
	Body map[string]interface{} `json:"body"`
}
type PendingTelemetryBatch struct {
	MessageID string `json:"messageId"`
	Readings []Reading `json:"readings"`
}
type PendingCommandAck struct {
	MessageID string `json:"messageId"`
	CommandID string `json:"commandId,omitempty"`
	Status int `json:"status"`
	Response map[string]interface{} `json:"response"`
}
type PersistedSpikeFault struct {
	Metric *string `json:"metric,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type PersistedFloodFault struct {
	Multiplier float64 `json:"multiplier"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type RuntimeSnapshot struct {
	DeviceTimeSec float64 `json:"deviceTimeSec"`
	Status string `json:"status"`
	OfflineExpiresAt *time.Time `json:"offlineExpiresAt,omitempty"`
	Spike *PersistedSpikeFault `json:"spike,omitempty"`
	Flood *PersistedFloodFault `json:"flood,omitempty"`
	OfflineRNGState *uint32 `json:"offlineRngState,omitempty"`
}
type PersistedMetricRuntime struct {
	X float64 `json:"x"`
	NextAt float64 `json:"nextAt"`
	Last *Reading `json:"last,omitempty"`
}
type TelemetrySnapshot struct {
	RNGState uint32 `json:"rngState"`
	Metrics map[string]PersistedMetricRuntime `json:"metrics"`
}
type Snapshot struct {
	Version int `json:"version"`
	DeviceID string `json:"deviceId"`
	State map[string]interface{} `json:"state"`
	Commands []CommandRecord `json:"commands"`
	Runtime *RuntimeSnapshot `json:"runtime,omitempty"`
	Telemetry *TelemetrySnapshot `json:"telemetry,omitempty"`
	PendingTelemetry []PendingTelemetryBatch `json:"pendingTelemetry,omitempty"`
	PendingCommandAcks []PendingCommandAck `json:"pendingCommandAcks,omitempty"`
}
type StateStore interface {
	Load(deviceID string) (Snapshot, error)
	Save(snapshot Snapshot) error
}

type Device struct {
	mu           sync.Mutex
	DeviceID     string
	Spec         *spec.DeviceSpec
	TimeScale    float64
	Rng          *prng.Mulberry32
	OfflineRng   *prng.Mulberry32
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
	PendingTelemetryBatches []PendingTelemetryBatch
	PendingCommandAckBatches []PendingCommandAck
	Listeners    []func(Event)
	Store StateStore
}

type Config struct {
	DeviceID  string
	Spec      *spec.DeviceSpec
	Seed      *int64
	TimeScale float64
	Store StateStore
}

func NewDevice(cfg Config) *Device {
	d, err := NewDeviceWithError(cfg)
	if err != nil { panic(err) }
	return d
}

func NewDeviceWithError(cfg Config) (*Device, error) {
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
		OfflineRng:  prng.NewMulberry32(seed ^ 0x9e3779b9),
		BootID:      generateUUID(),
		StartedAt:   time.Now(),
		VT:          0,
		Status:      "ONLINE",
		State:       make(map[string]interface{}),
		Metrics:     make(map[string]*MetricRuntime),
		Recent:      make(map[string]CachedCommand),
		RecentOrder: make([]string, 0, MaxRecent),
		Store: cfg.Store,
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

	if d.Store != nil {
		snapshot, err := d.Store.Load(d.DeviceID)
		if err != nil { return nil, fmt.Errorf("load persistent device state: %w", err) }
		if snapshot.Version != 1 { return nil, fmt.Errorf("unsupported device state snapshot version %d", snapshot.Version) }
		if snapshot.DeviceID != d.DeviceID { return nil, fmt.Errorf("device state belongs to %q, not %q", snapshot.DeviceID, d.DeviceID) }
		// A nil state means this is a new device with no saved snapshot yet.
		if snapshot.State != nil {
			for name := range snapshot.State {
				if _, ok := d.State[name]; !ok { return nil, fmt.Errorf("persisted state contains unknown field %q", name) }
			}
			for name := range d.State {
				if _, ok := snapshot.State[name]; !ok { return nil, fmt.Errorf("persisted state is missing field %q", name) }
			}
			d.State = snapshot.State
			for _, record := range snapshot.Commands {
				if record.ID == "" || len(d.RecentOrder) >= MaxRecent { return nil, fmt.Errorf("invalid or excessive persisted command history") }
				if _, exists := d.Recent[record.ID]; exists { return nil, fmt.Errorf("duplicate command ID %q in persisted state", record.ID) }
				d.Recent[record.ID] = CachedCommand{Status:record.Status, Body:record.Body}
				d.RecentOrder = append(d.RecentOrder, record.ID)
			}
		}
		if snapshot.Runtime != nil {
			if err := d.restoreRuntime(snapshot.Runtime); err != nil { return nil, fmt.Errorf("restore runtime state: %w", err) }
		}
		if snapshot.Telemetry != nil {
			if err := d.restoreTelemetry(snapshot.Telemetry); err != nil { return nil, fmt.Errorf("restore telemetry state: %w", err) }
		}
		if err := d.restorePendingTelemetry(snapshot.PendingTelemetry); err != nil {
			return nil, fmt.Errorf("restore pending telemetry: %w", err)
		}
		if err := d.restorePendingCommandAcks(snapshot.PendingCommandAcks); err != nil {
			return nil, fmt.Errorf("restore pending command acknowledgements: %w", err)
		}
	}
	return d, nil
}

func (d *Device) runtimeSnapshot() *RuntimeSnapshot {
	now:=time.Now().UTC()
	offlineRNGState := d.OfflineRng.State()
	runtime:=&RuntimeSnapshot{DeviceTimeSec:d.VT,Status:d.Status,OfflineRNGState:&offlineRNGState}
	if d.Status=="OFFLINE" { remaining:=(d.OfflineUntil-d.VT)/d.TimeScale; expires:=now.Add(time.Duration(math.Max(0,remaining)*float64(time.Second))); runtime.OfflineExpiresAt=&expires }
	if d.Spike!=nil { remaining:=(d.Spike.Until-d.VT)/d.TimeScale; expires:=now.Add(time.Duration(math.Max(0,remaining)*float64(time.Second))); runtime.Spike=&PersistedSpikeFault{Metric:d.Spike.Metric,ExpiresAt:expires} }
	if d.Flood!=nil { remaining:=(d.Flood.Until-d.VT)/d.TimeScale; expires:=now.Add(time.Duration(math.Max(0,remaining)*float64(time.Second))); runtime.Flood=&PersistedFloodFault{Multiplier:d.Flood.Multiplier,ExpiresAt:expires} }
	return runtime
}
func (d *Device) restoreRuntime(runtime *RuntimeSnapshot) error {
	if math.IsNaN(runtime.DeviceTimeSec)||math.IsInf(runtime.DeviceTimeSec,0)||runtime.DeviceTimeSec<0{return fmt.Errorf("invalid persisted device time")}
	if runtime.Status!="ONLINE"&&runtime.Status!="OFFLINE"{return fmt.Errorf("invalid persisted status %q",runtime.Status)}
	d.VT=runtime.DeviceTimeSec
	if runtime.OfflineRNGState != nil { d.OfflineRng.Restore(*runtime.OfflineRNGState) }
	if runtime.Status=="OFFLINE" { if runtime.OfflineExpiresAt==nil{return fmt.Errorf("offline runtime is missing its expiry")}; remaining:=runtime.OfflineExpiresAt.Sub(time.Now()).Seconds()*d.TimeScale; if remaining>0{d.Status="OFFLINE";d.OfflineUntil=d.VT+remaining} }
	if runtime.Spike!=nil&&runtime.Spike.ExpiresAt.After(time.Now()) { if runtime.Spike.Metric!=nil { if _,ok:=d.Metrics[*runtime.Spike.Metric];!ok{return fmt.Errorf("persisted spike references unknown metric %q",*runtime.Spike.Metric)} }; d.Spike=&SpikeFault{Metric:runtime.Spike.Metric,Until:d.VT+runtime.Spike.ExpiresAt.Sub(time.Now()).Seconds()*d.TimeScale} }
	if runtime.Flood!=nil&&runtime.Flood.ExpiresAt.After(time.Now()) { if math.IsNaN(runtime.Flood.Multiplier)||math.IsInf(runtime.Flood.Multiplier,0)||runtime.Flood.Multiplier<2||runtime.Flood.Multiplier>100{return fmt.Errorf("invalid persisted flood multiplier")}; d.Flood=&FloodFault{Multiplier:runtime.Flood.Multiplier,Until:d.VT+runtime.Flood.ExpiresAt.Sub(time.Now()).Seconds()*d.TimeScale} }
	return nil
}
func (d *Device) telemetrySnapshot() *TelemetrySnapshot {
	metrics := make(map[string]PersistedMetricRuntime, len(d.Metrics))
	for name, metric := range d.Metrics {
		var last *Reading
		if metric.Last != nil { copy := *metric.Last; last = &copy }
		metrics[name] = PersistedMetricRuntime{X: metric.X, NextAt: metric.NextAt, Last: last}
	}
	return &TelemetrySnapshot{RNGState: d.Rng.State(), Metrics: metrics}
}

func (d *Device) restoreTelemetry(snapshot *TelemetrySnapshot) error {
	if snapshot.Metrics == nil || len(snapshot.Metrics) != len(d.Metrics) { return fmt.Errorf("persisted telemetry metric set does not match device spec") }
	for name := range snapshot.Metrics { if _, ok := d.Metrics[name]; !ok { return fmt.Errorf("persisted telemetry contains unknown metric %q", name) } }
	for name, metric := range d.Metrics {
		persisted, ok := snapshot.Metrics[name]
		if !ok { return fmt.Errorf("persisted telemetry is missing metric %q", name) }
		if math.IsNaN(persisted.X) || math.IsInf(persisted.X, 0) || math.IsNaN(persisted.NextAt) || math.IsInf(persisted.NextAt, 0) || persisted.NextAt < 0 {
			return fmt.Errorf("persisted telemetry for metric %q contains invalid numeric state", name)
		}
		if persisted.Last != nil {
			if persisted.Last.Metric != name || persisted.Last.Unit != metric.Def.Unit || math.IsNaN(persisted.Last.Value) || math.IsInf(persisted.Last.Value, 0) {
				return fmt.Errorf("persisted last reading for metric %q is invalid", name)
			}
			if _, err := time.Parse(time.RFC3339Nano, persisted.Last.TS); err != nil { return fmt.Errorf("persisted last reading timestamp for metric %q is invalid: %w", name, err) }
			last := *persisted.Last; metric.Last = &last
		} else { metric.Last = nil }
		metric.X, metric.NextAt = persisted.X, persisted.NextAt
	}
	d.Rng.Restore(snapshot.RNGState)
	return nil
}

func (d *Device) restorePendingTelemetry(pending []PendingTelemetryBatch) error {
	if len(pending) > MaxPendingTelemetry {
		return fmt.Errorf("persisted pending telemetry exceeds capacity %d", MaxPendingTelemetry)
	}
	seen := make(map[string]struct{}, len(pending))
	for _, batch := range pending {
		if batch.MessageID == "" || len(batch.Readings) == 0 {
			return fmt.Errorf("persisted pending telemetry contains an invalid batch")
		}
		if _, exists := seen[batch.MessageID]; exists {
			return fmt.Errorf("persisted pending telemetry contains duplicate message ID %q", batch.MessageID)
		}
		seen[batch.MessageID] = struct{}{}
		if len(batch.Readings) > len(d.Metrics) {
			return fmt.Errorf("persisted pending telemetry batch exceeds the configured metric count")
		}
		seenMetrics := make(map[string]struct{}, len(batch.Readings))
		for _, reading := range batch.Readings {
			metric, ok := d.Metrics[reading.Metric]
			if !ok || reading.Unit != metric.Def.Unit || math.IsNaN(reading.Value) || math.IsInf(reading.Value, 0) {
				return fmt.Errorf("persisted pending telemetry contains an invalid reading for metric %q", reading.Metric)
			}
			if _, exists := seenMetrics[reading.Metric]; exists {
				return fmt.Errorf("persisted pending telemetry contains duplicate readings for metric %q", reading.Metric)
			}
			seenMetrics[reading.Metric] = struct{}{}
			if _, err := time.Parse(time.RFC3339Nano, reading.TS); err != nil {
				return fmt.Errorf("persisted pending telemetry has an invalid timestamp: %w", err)
			}
		}
	}
	d.PendingTelemetryBatches = clonePendingTelemetry(pending)
	return nil
}

func clonePendingTelemetry(pending []PendingTelemetryBatch) []PendingTelemetryBatch {
	cloned := make([]PendingTelemetryBatch, len(pending))
	for i, batch := range pending {
		cloned[i] = PendingTelemetryBatch{MessageID: batch.MessageID, Readings: append([]Reading(nil), batch.Readings...)}
	}
	return cloned
}

func (d *Device) PendingTelemetry() []PendingTelemetryBatch {
	d.mu.Lock()
	defer d.mu.Unlock()
	return clonePendingTelemetry(d.PendingTelemetryBatches)
}

// AcknowledgeTelemetry transfers ownership of a batch to the durable transport outbox.
func (d *Device) AcknowledgeTelemetry(messageID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	index := -1
	for i, batch := range d.PendingTelemetryBatches {
		if batch.MessageID == messageID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	previous := d.PendingTelemetryBatches
	next := append([]PendingTelemetryBatch(nil), previous[:index]...)
	next = append(next, previous[index+1:]...)
	d.PendingTelemetryBatches = next
	if d.Store != nil {
		if err := d.Store.Save(d.snapshot(d.State, d.Recent, d.RecentOrder)); err != nil {
			d.PendingTelemetryBatches = previous
			return fmt.Errorf("persist telemetry handoff: %w", err)
		}
	}
	return nil
}

func clonePendingCommandAcks(pending []PendingCommandAck) []PendingCommandAck {
	cloned := make([]PendingCommandAck, len(pending))
	for i, ack := range pending {
		body, _ := json.Marshal(ack.Response)
		var response map[string]interface{}
		_ = json.Unmarshal(body, &response)
		cloned[i] = PendingCommandAck{MessageID: ack.MessageID, CommandID: ack.CommandID, Status: ack.Status, Response: response}
	}
	return cloned
}

func (d *Device) restorePendingCommandAcks(pending []PendingCommandAck) error {
	if len(pending) > MaxPendingCommandAcks {
		return fmt.Errorf("persisted pending command acknowledgements exceed capacity %d", MaxPendingCommandAcks)
	}
	seen := make(map[string]struct{}, len(pending))
	for _, ack := range pending {
		if ack.MessageID == "" || ack.Status < 100 || ack.Status > 599 || ack.Response == nil {
			return fmt.Errorf("persisted pending command acknowledgement is invalid")
		}
		if _, exists := seen[ack.MessageID]; exists {
			return fmt.Errorf("persisted pending command acknowledgements contain duplicate message ID %q", ack.MessageID)
		}
		seen[ack.MessageID] = struct{}{}
	}
	d.PendingCommandAckBatches = clonePendingCommandAcks(pending)
	return nil
}

func (d *Device) PendingCommandAcks() []PendingCommandAck {
	d.mu.Lock()
	defer d.mu.Unlock()
	return clonePendingCommandAcks(d.PendingCommandAckBatches)
}

// QueueCommandAck durably records an inbound MQTT command result before the
// broker delivery is acknowledged.
func (d *Device) QueueCommandAck(commandID string, status int, response map[string]interface{}) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if commandID != "" {
		for _, ack := range d.PendingCommandAckBatches {
			if ack.CommandID == commandID {
				return ack.MessageID, nil
			}
		}
	}
	if len(d.PendingCommandAckBatches) >= MaxPendingCommandAcks {
		return "", fmt.Errorf("pending command acknowledgement queue is full")
	}
	messageID := generateUUID()
	ack := PendingCommandAck{MessageID: messageID, CommandID: commandID, Status: status, Response: response}
	d.PendingCommandAckBatches = append(d.PendingCommandAckBatches, ack)
	if d.Store != nil {
		if err := d.Store.Save(d.snapshot(d.State, d.Recent, d.RecentOrder)); err != nil {
			d.PendingCommandAckBatches = d.PendingCommandAckBatches[:len(d.PendingCommandAckBatches)-1]
			return "", fmt.Errorf("persist pending command acknowledgement: %w", err)
		}
	}
	return messageID, nil
}

func (d *Device) AcknowledgeCommandAck(messageID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	index := -1
	for i, ack := range d.PendingCommandAckBatches {
		if ack.MessageID == messageID { index = i; break }
	}
	if index < 0 { return nil }
	previous := d.PendingCommandAckBatches
	next := append([]PendingCommandAck(nil), previous[:index]...)
	next = append(next, previous[index+1:]...)
	d.PendingCommandAckBatches = next
	if d.Store != nil {
		if err := d.Store.Save(d.snapshot(d.State, d.Recent, d.RecentOrder)); err != nil {
			d.PendingCommandAckBatches = previous
			return fmt.Errorf("persist command acknowledgement handoff: %w", err)
		}
	}
	return nil
}

func (d *Device) persistRuntimeLocked() error { if d.Store==nil{return nil}; return d.Store.Save(d.snapshot(d.State,d.Recent,d.RecentOrder)) }
func (d *Device) snapshot(state map[string]interface{}, recent map[string]CachedCommand, order []string) Snapshot {
	records := make([]CommandRecord, 0, len(order))
	for _, id := range order {
		if c, ok := recent[id]; ok { records = append(records, CommandRecord{ID:id, Status:c.Status, Body:c.Body}) }
	}
	return Snapshot{Version:1, DeviceID:d.DeviceID, State:state, Commands:records, Runtime:d.runtimeSnapshot(), Telemetry:d.telemetrySnapshot(), PendingTelemetry:clonePendingTelemetry(d.PendingTelemetryBatches), PendingCommandAcks:clonePendingCommandAcks(d.PendingCommandAckBatches)}
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

	previousVT := d.VT
	previousStatus := d.Status
	previousOfflineUntil := d.OfflineUntil
	previousSpike := d.Spike
	previousFlood := d.Flood
	previousTelemetry := d.telemetrySnapshot()
	previousPendingTelemetry := clonePendingTelemetry(d.PendingTelemetryBatches)
	previousOfflineRNGState := d.OfflineRng.State()
	rollback := func() {
		d.VT = previousVT
		d.Status = previousStatus
		d.OfflineUntil = previousOfflineUntil
		d.Spike = previousSpike
		d.Flood = previousFlood
		_ = d.restoreTelemetry(previousTelemetry)
		d.PendingTelemetryBatches = previousPendingTelemetry
		d.OfflineRng.Restore(previousOfflineRNGState)
	}

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
			d.Status, d.OfflineUntil = "ONLINE", 0
			for _, metric := range d.Metrics {
				metric.NextAt = d.VT
			}
			// Persist recovery and the reset emission schedule atomically.
			if err := d.persistRuntimeLocked(); err != nil {
				rollback()
				return nil
			}
			d.emit(Event{Type: "STATUS_CHANGED", Status: "ONLINE", Reason: "RECOVERED"})
		}
		return nil
	}

	if d.Spec.OfflineRate > 0 {
		p := 1.0 - math.Pow(1.0-d.Spec.OfflineRate, dt)
		if d.OfflineRng.Next() < p {
			d.Status = "OFFLINE"
			d.OfflineUntil = d.VT + 5.0 + d.OfflineRng.Next()*15.0
			if err := d.persistRuntimeLocked(); err != nil {
				rollback()
				return nil
			}
			d.emit(Event{Type: "STATUS_CHANGED", Status: "OFFLINE", Reason: "RANDOM"})
			return nil
		}
	}

	if len(d.PendingTelemetryBatches) >= MaxPendingTelemetry {
		return nil
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

	// Persist each generated batch locally before handing it to the transport.
	// The stable ID allows a crash between outbox enqueue and local acknowledgement
	// to be replayed without creating a new logical message.
	if len(readings) > 0 {
		batch := PendingTelemetryBatch{MessageID: generateUUID(), Readings: append([]Reading(nil), readings...)}
		d.PendingTelemetryBatches = append(d.PendingTelemetryBatches, batch)
		if d.Store != nil {
			if err := d.Store.Save(d.snapshot(d.State, d.Recent, d.RecentOrder)); err != nil {
				rollback()
				return nil
			}
		}
	}

	return readings
}

func (d *Device) remember(commandId string, status int, body map[string]interface{}) (int, map[string]interface{}) {
	next := make(map[string]CachedCommand, len(d.Recent)+1)
	for id, c := range d.Recent { next[id] = c }
	order := append([]string(nil), d.RecentOrder...)
	if len(order) >= MaxRecent { delete(next, order[0]); order = order[1:] }
	next[commandId] = CachedCommand{Status:status, Body:body}
	order = append(order, commandId)
	if d.Store != nil {
		if err := d.Store.Save(d.snapshot(d.State, next, order)); err != nil {
			return 503, map[string]interface{}{"commandId":commandId,"status":"REJECTED","reason":"PERSISTENCE_FAILED","message":"command result could not be persisted; retry with the same commandId"}
		}
	}
	d.Recent, d.RecentOrder = next, order
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

	nextState := make(map[string]interface{}, len(d.State))
	for k, v := range d.State {
		nextState[k] = v
	}
	for field, val := range targetCmd.Effects {
		if strVal, isStr := val.(string); isStr && len(strVal) > 1 && strVal[0] == '$' {
			nextState[field] = params[strVal[1:]]
		} else {
			nextState[field] = val
		}
	}
	beforeBytes, _ := json.Marshal(d.State)
	afterBytes, _ := json.Marshal(nextState)
	changed := string(beforeBytes) != string(afterBytes)

	stateCopy := make(map[string]interface{})
	for k, v := range nextState {
		stateCopy[k] = v
	}
	resBody := map[string]interface{}{
		"commandId": cmdID,
		"status":    "EXECUTED",
		"state":     stateCopy,
		"changed":   changed,
	}

	nextRecent := make(map[string]CachedCommand, len(d.Recent)+1)
	for id, cached := range d.Recent {
		nextRecent[id] = cached
	}
	nextOrder := append([]string(nil), d.RecentOrder...)
	if len(nextOrder) >= MaxRecent {
		delete(nextRecent, nextOrder[0])
		nextOrder = nextOrder[1:]
	}
	nextRecent[cmdID] = CachedCommand{Status: 200, Body: resBody}
	nextOrder = append(nextOrder, cmdID)

	// Commit state and command result together before acknowledging execution.
	if d.Store != nil {
		if err := d.Store.Save(d.snapshot(nextState, nextRecent, nextOrder)); err != nil {
			return 503, map[string]interface{}{
				"commandId": cmdID,
				"status":    "REJECTED",
				"reason":    "PERSISTENCE_FAILED",
				"message":   "command result could not be persisted; retry with the same commandId",
			}
		}
	}
	d.State = nextState
	d.Recent = nextRecent
	d.RecentOrder = nextOrder

	if changed {
		d.emit(Event{
			Type:  "STATE_CHANGED",
			State: stateCopy,
			Cause: map[string]interface{}{"commandId": cmdID},
		})
	}
	return 200, resBody
}

func (d *Device) ApplyFault(raw []byte) (int, map[string]interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var req map[string]interface{}
	if err:=json.Unmarshal(raw,&req);err!=nil{return 400,map[string]interface{}{"error":"action is required (OFFLINE | SPIKE | FLOOD | CLEAR)"}}
	action,ok:=req["action"].(string);if !ok{return 400,map[string]interface{}{"error":"action is required (OFFLINE | SPIKE | FLOOD | CLEAR)"}}
	var duration *float64
	if value,exists:=req["durationSec"];exists{number,ok:=value.(float64);if !ok||number<=0||number>86400{return 400,map[string]interface{}{"error":"durationSec must be a number between 0 and 86400"}};duration=&number}
	durationOrDefault:=func(def float64)float64{if duration!=nil{return *duration};return def}
	oldStatus,oldOfflineUntil,oldSpike,oldFlood:=d.Status,d.OfflineUntil,d.Spike,d.Flood
	reason:=""
	switch action {
	case "OFFLINE":
		d.Status="OFFLINE";d.OfflineUntil=d.VT+durationOrDefault(60);if oldStatus!="OFFLINE"{reason="FAULT_INJECTED"}
	case "SPIKE":
		var metricPtr *string
		if value,exists:=req["metric"];exists{name,ok:=value.(string);if !ok{return 400,map[string]interface{}{"error":"metric must be a string"}};if _,exists:=d.Metrics[name];!exists{return 400,map[string]interface{}{"error":fmt.Sprintf("unknown metric '%s'",name)}};metricPtr=&name}
		d.Spike=&SpikeFault{Metric:metricPtr,Until:d.VT+durationOrDefault(30)}
	case "FLOOD":
		multiplier:=10.0
		if value,exists:=req["multiplier"];exists{number,ok:=value.(float64);if !ok||number<2||number>100{return 400,map[string]interface{}{"error":"multiplier must be between 2 and 100"}};multiplier=number}
		d.Flood=&FloodFault{Multiplier:multiplier,Until:d.VT+durationOrDefault(30)}
	case "CLEAR":
		d.Spike,d.Flood=nil,nil
		if d.Status=="OFFLINE"{d.Status,d.OfflineUntil="ONLINE",0;reason="FAULT_CLEARED"}
	default:return 400,map[string]interface{}{"error":fmt.Sprintf("unknown action '%s'",action)}
	}
	if err:=d.persistRuntimeLocked();err!=nil{d.Status,d.OfflineUntil,d.Spike,d.Flood=oldStatus,oldOfflineUntil,oldSpike,oldFlood;return 503,map[string]interface{}{"error":"runtime state could not be persisted","reason":"PERSISTENCE_FAILED"}}
	if reason!=""{if d.Status=="ONLINE"{for _,metric:=range d.Metrics{metric.NextAt=d.VT}};d.emit(Event{Type:"STATUS_CHANGED",Status:d.Status,Reason:reason})}
	switch action {
	case "OFFLINE":return 200,map[string]interface{}{"ok":true,"action":"OFFLINE"}
	case "SPIKE":result:=map[string]interface{}{"ok":true,"action":"SPIKE"};if d.Spike.Metric!=nil{result["metric"]=*d.Spike.Metric};return 200,result
	case "FLOOD":return 200,map[string]interface{}{"ok":true,"action":"FLOOD","multiplier":d.Flood.Multiplier}
	default:return 200,map[string]interface{}{"ok":true,"action":"CLEAR"}
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
		"pendingTelemetryBatches": len(d.PendingTelemetryBatches),
		"pendingCommandAcks": len(d.PendingCommandAckBatches),
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

func (d *Device) StatusValue() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Status
}
