---
id: device-api
title: Device API Reference
sidebar_label: Device API
sidebar_position: 1
---

# Device API Reference

Every device container exposes an HTTP API on its configured `PORT` (default `8080`). This API is used by the Fog Gateway and platform services to query and control the device.

## Authentication

If `DEVICE_TOKEN` is set, all endpoints (except `/health`) require the header:

```
X-Device-Token: <token>
```

Missing or wrong token → `401 Unauthorized`:

```json
{ "error": "unauthorized" }
```

## Endpoints

### `GET /health`

Health check endpoint. **Does not require authentication.**

**Response (200):**
```json
{
  "status": "UP",
  "deviceId": "light-1",
  "bootId": "550e8400-e29b-41d4-a716-446655440000",
  "registered": true
}
```

---

### `GET /info`

Returns the device's self-description, including its telemetry, state, and command definitions.

**Response (200):**
```json
{
  "deviceId": "light-1",
  "bootId": "550e8400-...",
  "runtimeVersion": "1.0.0",
  "type": "smart-light",
  "version": 1,
  "description": "Dimmable smart light bulb",
  "status": "ONLINE",
  "telemetry": [
    { "name": "brightness_sensor", "unit": "lux", "min": 0, "max": 1000, "intervalSec": 5 }
  ],
  "state": [
    { "name": "power", "type": "enum", "values": ["ON", "OFF"], "initial": "OFF" },
    { "name": "brightness", "type": "number", "min": 0, "max": 100, "initial": 0 }
  ],
  "commands": [
    {
      "name": "SET_BRIGHTNESS",
      "params": { "level": { "type": "number", "min": 0, "max": 100 } },
      "userControllable": true
    }
  ]
}
```

---

### `GET /state`

Returns the device's current state, status, and latest telemetry readings.

**Response (200):**
```json
{
  "deviceId": "light-1",
  "bootId": "550e8400-...",
  "status": "ONLINE",
  "state": {
    "power": "ON",
    "brightness": 75
  },
  "lastTelemetry": {
    "brightness_sensor": { "value": 420.5, "ts": "2025-01-15T10:30:00.000Z" }
  },
  "uptimeSec": 3600
}
```

---

### `POST /commands`

Execute a command on the device.

**Request:**
```json
{
  "commandId": "unique-id-001",
  "name": "SET_BRIGHTNESS",
  "params": { "level": 75 }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `commandId` | `string` | ✅ | Unique identifier (1 to 100 chars) for idempotency |
| `name` | `string` | ✅ | Command name from the spec |
| `params` | `object` | ❌ | Command parameters (required if the command has params) |

**Successful Response (200):**
```json
{
  "commandId": "unique-id-001",
  "status": "EXECUTED",
  "state": { "power": "ON", "brightness": 75 },
  "changed": true
}
```

**Duplicate Response (200):**
```json
{
  "commandId": "unique-id-001",
  "status": "EXECUTED",
  "state": { "power": "ON", "brightness": 75 },
  "changed": true,
  "duplicate": true
}
```

**Error Responses:**

| Status | Reason | When |
|---|---|---|
| `400` | `INVALID_REQUEST` | Missing/invalid `commandId` or `name` |
| `400` | `UNKNOWN_COMMAND` | Command name not found in spec |
| `400` | `INVALID_PARAMS` | Missing, extra, or invalid params |
| `503` | `DEVICE_OFFLINE` | Device is currently offline |

**Error Response (400):**
```json
{
  "commandId": "...",
  "status": "REJECTED",
  "reason": "INVALID_PARAMS",
  "message": "missing param 'level'"
}
```

---

## Control API (Instructor-Only)

The Control API is enabled when `CONTROL_TOKEN` is set. It uses a separate authentication header:

```
X-Control-Token: <token>
```

### `GET /control/status`

Returns internal device and gateway client status.

**Response (200):**
```json
{
  "deviceId": "sensor-1",
  "status": "ONLINE",
  "deviceTimeSec": 142.5,
  "offlineUntil": null,
  "spike": null,
  "flood": null,
  "gateway": {
    "registered": true,
    "queued": 0,
    "dropped": 0,
    "seq": 85
  }
}
```

### `POST /control/fault`

Inject a fault into the device simulation.

**Actions:**

#### OFFLINE

```json
{ "action": "OFFLINE", "durationSec": 120 }
```

Forces the device offline for the specified duration (device time). Default: 60s.

#### SPIKE

```json
{ "action": "SPIKE", "metric": "temperature", "durationSec": 30 }
```

Forces out-of-range values. Omit `metric` to spike all metrics. Default: 30s.

#### FLOOD

```json
{ "action": "FLOOD", "multiplier": 10, "durationSec": 30 }
```

Increases telemetry emission rate. Multiplier range: 2 to 100. Default: multiplier=10, duration=30s.

#### CLEAR

```json
{ "action": "CLEAR" }
```

Clears all active faults and recovers from offline.

**Response (200):**
```json
{ "ok": true, "action": "OFFLINE" }
```

:::note
The Control API does **not** use `DEVICE_TOKEN`: it has its own token. Without `CONTROL_TOKEN`, all `/control/*` paths return `404`.
:::
