---
id: mock-gateway
title: Mock Gateway
sidebar_label: Mock Gateway
sidebar_position: 2
---

# Mock Gateway

The `cmd/mock-gateway` is a testing tool written in Go that simulates the Fog Gateway. Use it to verify device behavior before building your own gateway.

:::warning
The mock gateway is **not** a reference implementation. It does not perform aggregation, buffering, or broker communication. It is a testing and validation tool only.
:::

## Starting the Mock Gateway

```bash
go run ./cmd/mock-gateway --port 8081
```

### Options

| Flag | Default | Description |
|---|---|---|
| `--port` | `8081` | Listen port |
| `--device-token` | _(none)_ | Require this `X-Device-Token` from devices |
| `--check` | _(off)_ | Run conformance checks then exit |
| `--control-token` | _(none)_ | Also test the `/control` API in check mode |
| `--device-url` | _(auto)_ | Override the device endpoint URL in check mode |
| `--timeout` | `60` | Seconds to wait for a device to register (check mode) |
| `--quiet` | _(off)_ | Suppress telemetry message logging |

## What It Does

The mock gateway:

1. **Accepts registrations** at `POST /devices/register`
2. **Accepts telemetry** at `POST /devices/{id}/telemetry`
3. **Accepts events** at `POST /devices/{id}/events`
4. **Validates** every payload against the contract
5. **Tracks** sequence numbers, duplicates, gaps, and out-of-order messages
6. **Logs** everything to the console

## Debug Endpoint

Query the gateway to see what it has received:

```bash
curl http://localhost:8081/debug/devices
```

**Response:**
```json
{
  "sensor-1": {
    "bootId": "...",
    "endpoint": "http://127.0.0.1:8080",
    "type": "temp-sensor",
    "status": "ONLINE",
    "state": {},
    "telemetryMessages": 42,
    "events": 3,
    "duplicates": 0,
    "gaps": 0,
    "lastSeq": 45
  }
}
```

## Conformance Mode (`--check`)

The `--check` flag turns the mock gateway into a conformance test runner. It waits for a device to register, then exercises the device's API and reports PASS/FAIL for each check.

### Running Conformance Checks

**Terminal 1: Start the gateway in check mode:**
```bash
go run ./cmd/mock-gateway --port 8081 --device-token t --check
```

**Terminal 2: Start a device:**
```bash
DEVICE_ID=d1 \
GATEWAY_URL=http://127.0.0.1:8081 \
DEVICE_TOKEN=t \
ADVERTISE_URL=http://127.0.0.1:8080 \
DEVICE_SPEC_FILE=examples/thermostat.yaml \
go run .
```

### What Gets Checked

| Check | Description |
|---|---|
| Register payload | Registration body matches the contract |
| `GET /health` | Returns `200` with `status: "UP"`, no auth needed |
| `GET /info` | Returns `200` with self-describing data |
| `/info` schema | Contains type, version, telemetry[], state[], commands[] |
| `userControllable` | Every command declares userControllable as boolean |
| `GET /state` | Returns `200` with status, state, lastTelemetry |
| Auth enforcement | Missing/wrong token → `401` (if token is configured) |
| Telemetry delivery | Telemetry arrives at the gateway |
| Metric validation | Only declared metrics with numeric values |
| Command execution | Each command returns `200 EXECUTED` with state |
| `STATE_CHANGED` event | State-changing commands trigger events |
| Idempotency | Repeated commandId → `200` with `duplicate: true` |
| No duplicate events | Duplicate commands don't emit extra events |
| Unknown command | → `400 REJECTED/UNKNOWN_COMMAND` |
| Missing commandId | → `400 INVALID_REQUEST` |
| Missing param | → `400 INVALID_PARAMS` |
| Invalid param value | → `400 INVALID_PARAMS` |
| Sequence ordering | `seq` is unique and increasing, no gaps or duplicates |

### With Control Token

Add `--control-token` to also test the instructor Control API:

```bash
go run ./cmd/mock-gateway --port 8081 --device-token t --control-token c --check
```

Additional checks:
- Wrong control token → `401`
- `OFFLINE` fault → device sends `STATUS_CHANGED(OFFLINE)`
- Commands rejected with `503` while offline
- `CLEAR` fault → device sends `STATUS_CHANGED(ONLINE)`

### Exit Code

- `0`: All checks passed
- `1`: One or more checks failed
