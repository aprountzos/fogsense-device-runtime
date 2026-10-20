---
id: gateway-api
title: Gateway API Reference
sidebar_label: Gateway API
sidebar_position: 2
---

# Gateway API Reference

The Fog Gateway must implement the following HTTP endpoints. The device runtime calls these endpoints to register itself, send telemetry, and report events.

The full OpenAPI specification is available at `contract/gateway-api.openapi.yaml`.

## Endpoints Your Gateway Must Implement

### `POST /devices/register`

Called at startup and periodically as a heartbeat.

**Request Body:**
```json
{
  "deviceId": "sensor-1",
  "bootId": "550e8400-e29b-41d4-a716-446655440000",
  "endpoint": "http://sensor-1:8080",
  "info": {
    "deviceId": "sensor-1",
    "bootId": "...",
    "runtimeVersion": "1.0.0",
    "type": "temp-sensor",
    "version": 1,
    "description": "Temperature and humidity sensor",
    "status": "ONLINE",
    "telemetry": [ ... ],
    "state": [ ... ],
    "commands": [ ... ]
  },
  "state": { },
  "status": "ONLINE",
  "ts": "2025-01-15T10:00:00.000Z"
}
```

**Expected Responses:**

| Status | Meaning | Device Action |
|---|---|---|
| `2xx` | ✅ Registration accepted | Marks as registered, schedules next heartbeat |
| Other | ❌ Registration failed | Retries with exponential backoff |

**Headers sent by the device:**

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `X-Device-Id` | The device's ID |
| `X-Device-Token` | Shared secret (if configured) |

---

### `POST /devices/{id}/telemetry`

Sends a batch of telemetry readings.

**Request Body:**
```json
{
  "deviceId": "sensor-1",
  "bootId": "550e8400-...",
  "seq": 42,
  "ts": "2025-01-15T10:30:00.000Z",
  "readings": [
    { "metric": "temperature", "value": 22.35, "unit": "°C", "ts": "2025-01-15T10:30:00.000Z" },
    { "metric": "humidity", "value": 45.12, "unit": "%", "ts": "2025-01-15T10:30:00.000Z" }
  ]
}
```

**Expected Responses:**

| Status | Meaning | Device Action |
|---|---|---|
| `2xx` | ✅ Accepted | Message dequeued |
| `404` | Device unknown | Re-register, then retry |
| `429` / `5xx` | Overloaded or error | Retry with backoff |
| Other `4xx` | Invalid message | Message dropped (logged) |

---

### `POST /devices/{id}/events`

Reports a device event (state change or status change).

**STATE_CHANGED Event:**
```json
{
  "deviceId": "light-1",
  "bootId": "550e8400-...",
  "seq": 15,
  "ts": "2025-01-15T10:30:00.000Z",
  "eventId": "evt-uuid",
  "type": "STATE_CHANGED",
  "state": { "power": "ON", "brightness": 75 },
  "cause": { "commandId": "cmd-001" }
}
```

**STATUS_CHANGED Event:**
```json
{
  "deviceId": "sensor-1",
  "bootId": "550e8400-...",
  "seq": 16,
  "ts": "2025-01-15T10:30:05.000Z",
  "eventId": "evt-uuid-2",
  "type": "STATUS_CHANGED",
  "status": "OFFLINE",
  "reason": "RANDOM"
}
```

**Expected responses** are the same as for telemetry (see table above).

## Ordering Contract

The `(bootId, seq)` pair provides ordering guarantees:

1. `seq` is **strictly increasing** per boot, shared across telemetry and events
2. Messages are sent **in FIFO order**: the device does not send message `n+1` until message `n` is acknowledged
3. A new `bootId` means the device restarted; the gateway should reset its sequence tracking

### Duplicate Handling

If the gateway responds `2xx` but the network drops the response, the device will retry. Your gateway should:

- Track the last seen `seq` per `(deviceId, bootId)`
- Accept and de-duplicate retransmissions
- Optionally return `{ "ok": true, "duplicate": true }`

### Gap Detection

If `seq` jumps (e.g., 41 -> 44), messages 42-43 were lost (dropped from the device's queue during an extended outage). The gateway should log this but continue processing.
