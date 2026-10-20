package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type DeviceRecord struct {
	BootID         string                   `json:"bootId"`
	Endpoint       string                   `json:"endpoint"`
	Info           map[string]interface{}   `json:"info"`
	Type           string                   `json:"type"`
	Status         string                   `json:"status"`
	State          map[string]interface{}   `json:"state"`
	TelemetryCount int                      `json:"telemetryMessages"`
	EventsCount    int                      `json:"events"`
	Duplicates     int                      `json:"duplicates"`
	Gaps           int                      `json:"gaps"`
	OutOfOrder     int                      `json:"outOfOrder,omitempty"`
	LastSeq        int64                    `json:"lastSeq"`
	Seen           map[int64]bool           `json:"-"`
	MaxSeq         int64                    `json:"-"`
	Telemetry      []map[string]interface{} `json:"-"`
	Events         []map[string]interface{} `json:"-"`
}

var (
	mu          sync.Mutex
	devices     = make(map[string]*DeviceRecord)
	quiet       bool
	deviceToken string
)

func logTime() string {
	return time.Now().UTC().Format("15:04:05.000")
}

func logMsg(format string, v ...interface{}) {
	fmt.Printf("[%s] %s\n", logTime(), fmt.Sprintf(format, v...))
}

func sendJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	body, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"INTERNAL"}`))
		return
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func readJSON(r *http.Request) (map[string]interface{}, error) {
	defer r.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
	if err != nil {
		return nil, err
	}
	if len(bodyBytes) == 0 {
		return make(map[string]interface{}), nil
	}
	var res map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func tokenOk(r *http.Request) bool {
	if deviceToken == "" {
		return true
	}
	return r.Header.Get("X-Device-Token") == deviceToken
}

// Validation helpers
func validateEnvelope(b map[string]interface{}, errors *[]string) {
	if devID, ok := b["deviceId"].(string); !ok || devID == "" {
		*errors = append(*errors, "deviceId (string) is required")
	}
	if bootID, ok := b["bootId"].(string); !ok || bootID == "" {
		*errors = append(*errors, "bootId (string) is required")
	}
	seqVal, hasSeq := b["seq"]
	if !hasSeq {
		*errors = append(*errors, "seq (integer >= 1) is required")
	} else {
		switch v := seqVal.(type) {
		case float64:
			if v < 1 || math.Floor(v) != v {
				*errors = append(*errors, "seq (integer >= 1) is required")
			}
		default:
			*errors = append(*errors, "seq (integer >= 1) is required")
		}
	}
	if ts, ok := b["ts"].(string); !ok || ts == "" {
		*errors = append(*errors, "ts (ISO string) is required")
	}
}

func validateRegister(b map[string]interface{}) []string {
	var errors []string
	if devID, ok := b["deviceId"].(string); !ok || devID == "" {
		errors = append(errors, "deviceId (string) is required")
	}
	if bootID, ok := b["bootId"].(string); !ok || bootID == "" {
		errors = append(errors, "bootId (string) is required")
	}
	endpoint, ok := b["endpoint"].(string)
	if !ok || (!strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://")) {
		errors = append(errors, "endpoint (http URL) is required")
	}
	status, ok := b["status"].(string)
	if !ok || (status != "ONLINE" && status != "OFFLINE") {
		errors = append(errors, "status must be ONLINE or OFFLINE")
	}
	if state, ok := b["state"].(map[string]interface{}); !ok || state == nil {
		errors = append(errors, "state (object) is required")
	}
	info, ok := b["info"].(map[string]interface{})
	if !ok || info == nil {
		errors = append(errors, "info (object) is required")
	} else {
		if t, ok := info["type"].(string); !ok || t == "" {
			errors = append(errors, "info.type is required")
		}
		if v, ok := info["version"].(float64); !ok || math.Floor(v) != v {
			errors = append(errors, "info.version (integer) is required")
		}
		for _, k := range []string{"telemetry", "state", "commands"} {
			if _, ok := info[k].([]interface{}); !ok {
				errors = append(errors, fmt.Sprintf("info.%s (array) is required", k))
			}
		}
	}
	return errors
}

func validateTelemetry(b map[string]interface{}) []string {
	var errors []string
	validateEnvelope(b, &errors)
	readings, ok := b["readings"].([]interface{})
	if !ok || len(readings) == 0 {
		errors = append(errors, "readings (non-empty array) is required")
	} else {
		for i, item := range readings {
			r, ok := item.(map[string]interface{})
			if !ok {
				errors = append(errors, fmt.Sprintf("readings[%d] must be an object", i))
				continue
			}
			if m, ok := r["metric"].(string); !ok || m == "" {
				errors = append(errors, fmt.Sprintf("readings[%d].metric is required", i))
			}
			val, hasVal := r["value"].(float64)
			if !hasVal || math.IsNaN(val) || math.IsInf(val, 0) {
				errors = append(errors, fmt.Sprintf("readings[%d].value must be a finite number", i))
			}
			if ts, ok := r["ts"].(string); !ok || ts == "" {
				errors = append(errors, fmt.Sprintf("readings[%d].ts is required", i))
			}
		}
	}
	return errors
}

func validateEvent(b map[string]interface{}) []string {
	var errors []string
	validateEnvelope(b, &errors)
	if eventID, ok := b["eventId"].(string); !ok || eventID == "" {
		errors = append(errors, "eventId (string) is required")
	}
	evType, ok := b["type"].(string)
	if !ok {
		errors = append(errors, "type must be STATE_CHANGED or STATUS_CHANGED")
		return errors
	}

	if evType == "STATE_CHANGED" {
		if s, ok := b["state"].(map[string]interface{}); !ok || s == nil {
			errors = append(errors, "state (object) is required for STATE_CHANGED")
		}
		if c, ok := b["cause"].(map[string]interface{}); !ok || c == nil {
			errors = append(errors, "cause (object) is required for STATE_CHANGED")
		}
	} else if evType == "STATUS_CHANGED" {
		status, ok := b["status"].(string)
		if !ok || (status != "ONLINE" && status != "OFFLINE") {
			errors = append(errors, "status must be ONLINE or OFFLINE")
		}
		if reason, ok := b["reason"].(string); !ok || reason == "" {
			errors = append(errors, "reason (string) is required for STATUS_CHANGED")
		}
	} else {
		errors = append(errors, "type must be STATE_CHANGED or STATUS_CHANGED")
	}
	return errors
}

func toInt64(val interface{}) int64 {
	switch v := val.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func handler() http.Handler {
	telemetryRegex := regexp.MustCompile(`^/devices/([^/]+)/telemetry$`)
	eventsRegex := regexp.MustCompile(`^/devices/([^/]+)/events$`)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if r.Method == http.MethodGet && path == "/debug/devices" {
			mu.Lock()
			out := make(map[string]map[string]interface{})
			for id, d := range devices {
				out[id] = map[string]interface{}{
					"bootId":            d.BootID,
					"endpoint":          d.Endpoint,
					"type":              d.Type,
					"status":            d.Status,
					"state":             d.State,
					"telemetryMessages": d.TelemetryCount,
					"events":            len(d.Events),
					"duplicates":        d.Duplicates,
					"gaps":              d.Gaps,
					"lastSeq":           d.MaxSeq,
				}
			}
			mu.Unlock()
			sendJSON(w, http.StatusOK, out)
			return
		}

		if r.Method != http.MethodPost {
			sendJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}

		body, err := readJSON(r)
		if err != nil {
			sendJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_JSON"})
			return
		}

		if path == "/devices/register" {
			if !tokenOk(r) {
				sendJSON(w, http.StatusForbidden, map[string]string{"error": "FORBIDDEN"})
				return
			}
			errs := validateRegister(body)
			if len(errs) > 0 {
				logMsg("REGISTER rejected: %v", errs)
				sendJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "INVALID_REQUEST", "details": errs})
				return
			}

			devID := body["deviceId"].(string)
			bootID := body["bootId"].(string)
			endpoint := body["endpoint"].(string)
			status := body["status"].(string)
			state := body["state"].(map[string]interface{})
			info := body["info"].(map[string]interface{})
			devType := info["type"].(string)
			version := info["version"].(float64)

			mu.Lock()
			existing, exists := devices[devID]
			var rec *DeviceRecord
			if exists && existing.BootID == bootID {
				rec = existing
				rec.Endpoint = endpoint
				rec.Info = info
				rec.Type = devType
				rec.State = state
				rec.Status = status
			} else {
				rec = &DeviceRecord{
					BootID:    bootID,
					Endpoint:  endpoint,
					Info:      info,
					Type:      devType,
					Status:    status,
					State:     state,
					Seen:      make(map[int64]bool),
					Telemetry: make([]map[string]interface{}, 0),
					Events:    make([]map[string]interface{}, 0),
				}
				devices[devID] = rec
				logMsg("REGISTER %s (%s v%g) at %s [new boot]", devID, devType, version, endpoint)
			}
			mu.Unlock()

			sendJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}

		// Check telemetry or events
		var devID string
		var kind string
		if m := telemetryRegex.FindStringSubmatch(path); len(m) > 1 {
			devID = m[1]
			kind = "telemetry"
		} else if m := eventsRegex.FindStringSubmatch(path); len(m) > 1 {
			devID = m[1]
			kind = "events"
		}

		if kind != "" {
			if !tokenOk(r) {
				sendJSON(w, http.StatusForbidden, map[string]string{"error": "FORBIDDEN"})
				return
			}
			var errs []string
			if kind == "telemetry" {
				errs = validateTelemetry(body)
			} else {
				errs = validateEvent(body)
			}
			if len(errs) > 0 {
				logMsg("%s rejected: %v", strings.ToUpper(kind), errs)
				sendJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "INVALID_REQUEST", "details": errs})
				return
			}

			bodyDevID, _ := body["deviceId"].(string)
			if bodyDevID != devID {
				sendJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error":   "INVALID_REQUEST",
					"details": []string{"deviceId does not match the URL"},
				})
				return
			}

			bootID, _ := body["bootId"].(string)
			seq := toInt64(body["seq"])

			mu.Lock()
			rec, exists := devices[devID]
			if !exists || rec.BootID != bootID {
				mu.Unlock()
				sendJSON(w, http.StatusNotFound, map[string]string{"error": "UNREGISTERED"})
				return
			}

			if rec.Seen[seq] {
				rec.Duplicates++
				mu.Unlock()
				sendJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "duplicate": true})
				return
			}

			if rec.MaxSeq > 0 && seq > rec.MaxSeq+1 {
				rec.Gaps++
			}
			if seq < rec.MaxSeq {
				rec.OutOfOrder++
			}
			rec.Seen[seq] = true
			if seq > rec.MaxSeq {
				rec.MaxSeq = seq
			}

			if kind == "telemetry" {
				rec.TelemetryCount++
				rec.Telemetry = append(rec.Telemetry, body)
				if !quiet {
					var readingsStr []string
					if readings, ok := body["readings"].([]interface{}); ok {
						for _, r := range readings {
							if rMap, ok := r.(map[string]interface{}); ok {
								readingsStr = append(readingsStr, fmt.Sprintf("%v=%v", rMap["metric"], rMap["value"]))
							}
						}
					}
					logMsg("TELEMETRY %s seq=%d %s", devID, seq, strings.Join(readingsStr, " "))
				}
			} else {
				rec.Events = append(rec.Events, body)
				evType, _ := body["type"].(string)
				if evType == "STATUS_CHANGED" {
					if st, ok := body["status"].(string); ok {
						rec.Status = st
					}
					logMsg("EVENT %s seq=%d STATUS_CHANGED %s (%v)", devID, seq, body["status"], body["reason"])
				} else if evType == "STATE_CHANGED" {
					if st, ok := body["state"].(map[string]interface{}); ok {
						rec.State = st
					}
					stateBytes, _ := json.Marshal(body["state"])
					logMsg("EVENT %s seq=%d STATE_CHANGED %s", devID, seq, string(stateBytes))
				}
			}
			mu.Unlock()

			sendJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}

		sendJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})

	return mux
}

// Conformance check runner
type checkResult struct {
	Name   string
	OK     bool
	Detail string
}

func waitFor(pred func() bool, timeout time.Duration, step time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(step)
	}
	return pred()
}

func httpCall(client *http.Client, base, method, path string, body interface{}, token string, extraHeaders map[string]string) (int, map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Device-Token", token)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	var respJSON map[string]interface{}
	if len(respBytes) > 0 {
		_ = json.Unmarshal(respBytes, &respJSON)
	}
	return resp.StatusCode, respJSON, nil
}

func sampleParams(paramDefs map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for name, defRaw := range paramDefs {
		def, ok := defRaw.(map[string]interface{})
		if !ok {
			out[name] = true
			continue
		}
		pType, _ := def["type"].(string)
		switch pType {
		case "enum":
			if vals, ok := def["values"].([]interface{}); ok && len(vals) > 0 {
				out[name] = vals[0]
			}
		case "number":
			minVal, _ := def["min"].(float64)
			maxVal, _ := def["max"].(float64)
			out[name] = (minVal + maxVal) / 2
		default:
			out[name] = true
		}
	}
	return out
}

func runChecks(port int, devToken, ctrlToken, overrideDevURL string, timeoutSec int) int {
	var results []checkResult
	check := func(name string, ok bool, detail ...string) {
		d := ""
		if len(detail) > 0 {
			d = detail[0]
		}
		results = append(results, checkResult{Name: name, OK: ok, Detail: d})
		if ok {
			fmt.Printf("  PASS  %s\n", name)
		} else {
			if d != "" {
				fmt.Printf("  FAIL  %s - %s\n", name, d)
			} else {
				fmt.Printf("  FAIL  %s\n", name)
			}
		}
	}

	timeout := time.Duration(timeoutSec) * time.Second
	logMsg("conformance mode: waiting up to %vs for a device to register on :%d ...", timeoutSec, port)

	hasDevice := waitFor(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(devices) > 0
	}, timeout, 100*time.Millisecond)

	if !hasDevice {
		fmt.Println("  FAIL  device registered with the gateway: none did within the timeout")
		return 1
	}

	mu.Lock()
	var devID string
	var rec *DeviceRecord
	for k, v := range devices {
		devID = k
		rec = v
		break
	}
	mu.Unlock()

	base := rec.Endpoint
	if overrideDevURL != "" {
		base = overrideDevURL
	}
	base = strings.TrimRight(base, "/")

	devType := rec.Type
	version := rec.Info["version"]
	fmt.Printf("\nChecking device '%s' (%s v%v) at %s\n\n", devID, devType, version, base)

	check("register payload matches the contract", true)

	client := &http.Client{Timeout: 5 * time.Second}

	// Health check
	status, body, _ := httpCall(client, base, "GET", "/health", nil, "", nil)
	check("GET /health -> 200 {status:\"UP\"} (no auth needed)", status == 200 && body != nil && body["status"] == "UP", fmt.Sprintf("got %d", status))

	// Info check
	status, body, _ = httpCall(client, base, "GET", "/info", nil, devToken, nil)
	check("GET /info -> 200", status == 200, fmt.Sprintf("got %d", status))
	info := body
	if info == nil {
		info = make(map[string]interface{})
	}

	hasSelfDescribe := info["type"] == rec.Info["type"]
	for _, k := range []string{"telemetry", "state", "commands"} {
		if _, ok := info[k].([]interface{}); !ok {
			hasSelfDescribe = false
		}
	}
	check("/info is self-describing (type, version, telemetry[], state[], commands[])", hasSelfDescribe)

	allUserControllable := true
	var commandsList []map[string]interface{}
	if cmdArr, ok := info["commands"].([]interface{}); ok {
		for _, c := range cmdArr {
			if cMap, ok := c.(map[string]interface{}); ok {
				commandsList = append(commandsList, cMap)
				if _, ok := cMap["userControllable"].(bool); !ok {
					allUserControllable = false
				}
			}
		}
	}
	check("every command in /info declares userControllable (boolean)", allUserControllable)

	// State check
	status, body, _ = httpCall(client, base, "GET", "/state", nil, devToken, nil)
	hasState := status == 200 && body != nil && body["status"] != nil
	if body != nil {
		_, okState := body["state"].(map[string]interface{})
		_, okLastTelem := body["lastTelemetry"].(map[string]interface{})
		hasState = hasState && okState && okLastTelem
	}
	check("GET /state -> 200 with status, state, lastTelemetry", hasState, fmt.Sprintf("got %d", status))

	// Token checks
	if devToken != "" {
		st, _, _ := httpCall(client, base, "GET", "/info", nil, "", nil)
		check("missing X-Device-Token -> 401", st == 401, fmt.Sprintf("got %d", st))

		st, _, _ = httpCall(client, base, "GET", "/info", nil, "wrong-token", nil)
		check("wrong X-Device-Token -> 401", st == 401, fmt.Sprintf("got %d", st))
	}

	// Telemetry check
	telemArr, _ := info["telemetry"].([]interface{})
	if len(telemArr) > 0 {
		gotTelem := waitFor(func() bool {
			mu.Lock()
			defer mu.Unlock()
			return rec.TelemetryCount > 0
		}, 25*time.Second, 100*time.Millisecond)
		check("telemetry arrives at the gateway", gotTelem)

		declaredMetrics := make(map[string]bool)
		for _, t := range telemArr {
			if tMap, ok := t.(map[string]interface{}); ok {
				if m, ok := tMap["name"].(string); ok {
					declaredMetrics[m] = true
				}
			}
		}

		mu.Lock()
		msgs := append([]map[string]interface{}(nil), rec.Telemetry...)
		mu.Unlock()

		validTelemetry := len(msgs) > 0
		if len(msgs) > 20 {
			msgs = msgs[:20]
		}
		for _, m := range msgs {
			readings, ok := m["readings"].([]interface{})
			if !ok || len(readings) == 0 {
				validTelemetry = false
				break
			}
			for _, rItem := range readings {
				rMap, ok := rItem.(map[string]interface{})
				if !ok {
					validTelemetry = false
					break
				}
				metricName, _ := rMap["metric"].(string)
				val, okVal := rMap["value"].(float64)
				if !declaredMetrics[metricName] || !okVal || math.IsNaN(val) || math.IsInf(val, 0) {
					validTelemetry = false
					break
				}
			}
		}
		check("telemetry only contains declared metrics with numeric values", validTelemetry)
	} else {
		mu.Lock()
		tCount := rec.TelemetryCount
		mu.Unlock()
		check("device without telemetry sends no telemetry", tCount == 0)
	}

	// Commands check
	var firstCommand map[string]interface{}
	var firstCommandID string
	var firstCmdParams map[string]interface{}

	for _, cmd := range commandsList {
		cmdName, _ := cmd["name"].(string)
		cmdParamsDef, _ := cmd["params"].(map[string]interface{})
		params := sampleParams(cmdParamsDef)
		cmdID := fmt.Sprintf("check-%s-%d", cmdName, time.Now().UnixNano())

		mu.Lock()
		eventsBefore := len(rec.Events)
		mu.Unlock()

		reqBody := map[string]interface{}{
			"commandId": cmdID,
			"name":      cmdName,
			"params":    params,
		}
		st, respB, _ := httpCall(client, base, "POST", "/commands", reqBody, devToken, nil)

		okExec := st == 200 && respB != nil && respB["status"] == "EXECUTED" && respB["commandId"] == cmdID
		if respB != nil {
			_, okSt := respB["state"].(map[string]interface{})
			_, okChg := respB["changed"].(bool)
			okExec = okExec && okSt && okChg
		}
		respJSONStr, _ := json.Marshal(respB)
		check(fmt.Sprintf("command %s -> 200 EXECUTED with resulting state", cmdName), okExec, fmt.Sprintf("got %d %s", st, string(respJSONStr)))

		if okExec && respB["changed"] == true {
			evArrived := waitFor(func() bool {
				mu.Lock()
				defer mu.Unlock()
				for _, e := range rec.Events[eventsBefore:] {
					if e["type"] == "STATE_CHANGED" {
						if cause, ok := e["cause"].(map[string]interface{}); ok {
							if cause["commandId"] == cmdID {
								return true
							}
						}
					}
				}
				return false
			}, 5*time.Second, 100*time.Millisecond)
			check(fmt.Sprintf("command %s changed the state -> STATE_CHANGED event reaches the gateway", cmdName), evArrived)
		}

		if firstCommand == nil {
			firstCommand = cmd
			firstCommandID = cmdID
			firstCmdParams = params
		}
		time.Sleep(50 * time.Millisecond)
	}

	if firstCommand != nil {
		mu.Lock()
		evBeforeDup := len(rec.Events)
		mu.Unlock()

		firstCmdName, _ := firstCommand["name"].(string)
		reqBody := map[string]interface{}{
			"commandId": firstCommandID,
			"name":      firstCmdName,
			"params":    firstCmdParams,
		}
		st, respB, _ := httpCall(client, base, "POST", "/commands", reqBody, devToken, nil)
		respJSONStr, _ := json.Marshal(respB)
		check("repeating a commandId is idempotent (200 + duplicate:true)", st == 200 && respB != nil && respB["duplicate"] == true, fmt.Sprintf("got %d %s", st, string(respJSONStr)))

		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		evAfterDup := len(rec.Events)
		mu.Unlock()
		check("a duplicate command does not emit another event", evAfterDup == evBeforeDup)

		// Unknown command
		st, respB, _ = httpCall(client, base, "POST", "/commands", map[string]interface{}{
			"commandId": fmt.Sprintf("check-unknown-%d", time.Now().UnixNano()),
			"name":      "DOES_NOT_EXIST",
		}, devToken, nil)
		respJSONStr, _ = json.Marshal(respB)
		check("unknown command -> 400 REJECTED/UNKNOWN_COMMAND", st == 400 && respB != nil && respB["status"] == "REJECTED" && respB["reason"] == "UNKNOWN_COMMAND", fmt.Sprintf("got %d %s", st, string(respJSONStr)))

		// Missing commandId
		st, respB, _ = httpCall(client, base, "POST", "/commands", map[string]interface{}{
			"name": firstCmdName,
		}, devToken, nil)
		check("missing commandId -> 400 INVALID_REQUEST", st == 400 && respB != nil && respB["reason"] == "INVALID_REQUEST", fmt.Sprintf("got %d", st))

		// Check param validation if any command has params
		var cmdWithParams map[string]interface{}
		for _, c := range commandsList {
			if p, ok := c["params"].(map[string]interface{}); ok && len(p) > 0 {
				cmdWithParams = c
				break
			}
		}

		if cmdWithParams != nil {
			cName, _ := cmdWithParams["name"].(string)
			pDefs, _ := cmdWithParams["params"].(map[string]interface{})

			st, respB, _ = httpCall(client, base, "POST", "/commands", map[string]interface{}{
				"commandId": fmt.Sprintf("check-noparams-%d", time.Now().UnixNano()),
				"name":      cName,
				"params":    map[string]interface{}{},
			}, devToken, nil)
			respJSONStr, _ = json.Marshal(respB)
			check("missing required param -> 400 INVALID_PARAMS", st == 400 && respB != nil && respB["reason"] == "INVALID_PARAMS", fmt.Sprintf("got %d %s", st, string(respJSONStr)))

			var paramName string
			var paramDef map[string]interface{}
			for pn, pd := range pDefs {
				paramName = pn
				paramDef, _ = pd.(map[string]interface{})
				break
			}

			badParams := sampleParams(pDefs)
			pType, _ := paramDef["type"].(string)
			if pType == "number" {
				maxVal, _ := paramDef["max"].(float64)
				badParams[paramName] = maxVal + 1000
			} else if pType == "enum" {
				badParams[paramName] = "__not_a_value__"
			} else {
				badParams[paramName] = "not-a-boolean"
			}

			st, respB, _ = httpCall(client, base, "POST", "/commands", map[string]interface{}{
				"commandId": fmt.Sprintf("check-badparam-%d", time.Now().UnixNano()),
				"name":      cName,
				"params":    badParams,
			}, devToken, nil)
			respJSONStr, _ = json.Marshal(respB)
			check("out-of-range/invalid param value -> 400 INVALID_PARAMS", st == 400 && respB != nil && respB["reason"] == "INVALID_PARAMS", fmt.Sprintf("got %d %s", st, string(respJSONStr)))
		}
	}

	// Ordering guarantees
	mu.Lock()
	dups := rec.Duplicates
	gaps := rec.Gaps
	ooo := rec.OutOfOrder
	mu.Unlock()
	check("seq is unique and increasing per boot (no duplicates received, no gaps)", dups == 0 && gaps == 0 && ooo == 0, fmt.Sprintf("duplicates=%d gaps=%d outOfOrder=%d", dups, gaps, ooo))

	// Control API
	if ctrlToken != "" {
		st, _, _ := httpCall(client, base, "GET", "/control/status", nil, "", map[string]string{"X-Control-Token": "wrong"})
		check("/control with a wrong token -> 401", st == 401, fmt.Sprintf("got %d", st))

		mu.Lock()
		evBeforeOff := len(rec.Events)
		mu.Unlock()

		st, _, _ = httpCall(client, base, "POST", "/control/fault", map[string]interface{}{
			"action":      "OFFLINE",
			"durationSec": 600,
		}, "", map[string]string{"X-Control-Token": ctrlToken})
		check("POST /control/fault OFFLINE -> 200", st == 200, fmt.Sprintf("got %d", st))

		offEv := waitFor(func() bool {
			mu.Lock()
			defer mu.Unlock()
			for _, e := range rec.Events[evBeforeOff:] {
				if e["type"] == "STATUS_CHANGED" && e["status"] == "OFFLINE" {
					return true
				}
			}
			return false
		}, 5*time.Second, 100*time.Millisecond)
		check("going OFFLINE sends STATUS_CHANGED(OFFLINE) to the gateway (last will)", offEv)

		if len(commandsList) > 0 {
			cName, _ := commandsList[0]["name"].(string)
			pDefs, _ := commandsList[0]["params"].(map[string]interface{})
			st, respB, _ := httpCall(client, base, "POST", "/commands", map[string]interface{}{
				"commandId": fmt.Sprintf("check-offline-%d", time.Now().UnixNano()),
				"name":      cName,
				"params":    sampleParams(pDefs),
			}, devToken, nil)
			respJSONStr, _ := json.Marshal(respB)
			check("an OFFLINE device rejects commands -> 503 DEVICE_OFFLINE", st == 503 && respB != nil && respB["reason"] == "DEVICE_OFFLINE", fmt.Sprintf("got %d %s", st, string(respJSONStr)))
		}

		mu.Lock()
		evBeforeOn := len(rec.Events)
		mu.Unlock()

		st, _, _ = httpCall(client, base, "POST", "/control/fault", map[string]interface{}{
			"action": "CLEAR",
		}, "", map[string]string{"X-Control-Token": ctrlToken})
		check("POST /control/fault CLEAR -> 200", st == 200, fmt.Sprintf("got %d", st))

		onEv := waitFor(func() bool {
			mu.Lock()
			defer mu.Unlock()
			for _, e := range rec.Events[evBeforeOn:] {
				if e["type"] == "STATUS_CHANGED" && e["status"] == "ONLINE" {
					return true
				}
			}
			return false
		}, 5*time.Second, 100*time.Millisecond)
		check("recovery sends STATUS_CHANGED(ONLINE)", onEv)
	} else {
		fmt.Println("  SKIP  /control checks (pass --control-token to include them)")
	}

	failed := 0
	for _, r := range results {
		if !r.OK {
			failed++
		}
	}
	if failed > 0 {
		fmt.Printf("\n%d/%d checks passed - %d FAILED\n", len(results)-failed, len(results), failed)
		return 1
	}
	fmt.Printf("\n%d/%d checks passed\n", len(results), len(results))
	return 0
}

func main() {
	port := flag.Int("port", 8081, "listen port")
	flag.StringVar(&deviceToken, "device-token", "", "require/expect this X-Device-Token")
	controlToken := flag.String("control-token", "", "control token for /control API checks")
	deviceURL := flag.String("device-url", "", "override the endpoint the device advertised")
	checkMode := flag.Bool("check", false, "run conformance checks, then exit")
	timeoutSec := flag.Int("timeout", 60, "seconds to wait for device in check mode")
	flag.BoolVar(&quiet, "quiet", false, "do not print every telemetry message")

	flag.Parse()

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: handler(),
	}

	go func() {
		tokenNote := ""
		if deviceToken != "" {
			tokenNote = " (device token required)"
		}
		logMsg("FogSense mock gateway listening on :%d%s", *port, tokenNote)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	if *checkMode {
		exitCode := runChecks(*port, deviceToken, *controlToken, *deviceURL, *timeoutSec)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		os.Exit(exitCode)
	}

	// Keep running in normal server mode
	select {}
}
