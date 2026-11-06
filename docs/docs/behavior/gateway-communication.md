---
id: gateway-communication
title: Gateway Communication
sidebar_label: Gateway Communication
sidebar_position: 3
---

# Gateway Communication

The device runtime supports HTTP (default) and MQTT 5.0 as alternative gateway transports. Both transports handle registration, telemetry delivery and event reporting. The device's local API remains HTTP in either mode.

## Select a transport

Set `GATEWAY_TRANSPORT=http` (the default) to use the existing HTTP gateway contract, or `GATEWAY_TRANSPORT=mqtt` to use an MQTT 5 broker.

| Setting | HTTP | MQTT |
|---|---|---|
| Gateway configuration | `GATEWAY_URL` | `MQTT_BROKER_URL` |
| Authentication | `GATEWAY_TOKEN` (falls back to `DEVICE_TOKEN`) | `MQTT_USERNAME` + `MQTT_PASSWORD` |
| Delivery | HTTP POST and response status | MQTT QoS 1 and application registration acknowledgement |
| Device API | HTTP | HTTP (unchanged) |

See the [MQTT 5 guide](../guides/mqtt.md) for broker setup, topic names, commands and security recommendations.

## Communication Endpoints

When HTTP is selected, the device calls three endpoints on the gateway:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/devices/register` | Register / heartbeat |
| `POST` | `/devices/{id}/telemetry` | Send telemetry batches |
| `POST` | `/devices/{id}/events` | Send events (state changes, status changes) |

The full gateway contract is defined in [contract/gateway-api.openapi.yaml](../../../contract/gateway-api.openapi.yaml), and the device-side API is documented in [contract/device-api.openapi.yaml](../../../contract/device-api.openapi.yaml).

## Registration & Heartbeat

The device registers with the gateway at startup and periodically re-registers as a heartbeat:

```json
{
  "deviceId": "sensor-1",
  "bootId": "uuid-...",
  "endpoint": "http://sensor-1:8080",
  "info": { /* full device info */ },
  "state": { "power": "ON" },
  "status": "ONLINE",
  "ts": "2025-01-15T10:00:00.000Z"
}
```

- Default heartbeat interval: **30 seconds** (`REGISTER_INTERVAL_SEC`)
- The `endpoint` tells the gateway how to reach this device's API
- While **OFFLINE**, no heartbeat is sent
- On recovery (`ONLINE`), an immediate re-registration is triggered

## Message Envelope

Every telemetry batch and event includes a standard envelope:

```json
{
  "deviceId": "sensor-1",
  "bootId": "abc-123",
  "seq": 42,
  "ts": "2025-01-15T10:30:00.000Z",
  // ... payload-specific fields
}
```

| Field | Description |
|---|---|
| `deviceId` | The device's unique identifier |
| `bootId` | UUID generated at startup, identifies this boot session |
| `seq` | Strictly increasing sequence number (shared across telemetry and events) |
| `ts` | ISO 8601 timestamp of the message |

The `(bootId, seq)` pair uniquely identifies each message and enables:
- **Duplicate detection**: the gateway can ignore messages with already-seen `seq`
- **Gap detection**: a jump in `seq` indicates missed messages
- **Boot detection**: a new `bootId` means the device restarted

## Delivery Guarantees

### At-Least-Once, In-Order

Messages are delivered with these guarantees:

1. Every message is delivered **at least once**
2. Messages are sent **in order** (the queue is FIFO)
3. A message is only removed from the queue after a successful `2xx` response

### Retry Strategy

| Gateway Response | Runtime Action |
|---|---|
| `2xx` | ✅ Message accepted, dequeued |
| `404` | Gateway doesn't know this device. Re-register, then retry |
| `429` / `5xx` | Retry with exponential backoff |
| Other `4xx` | Message is **dropped** (invalid or forbidden, logged) |
| Network error | Retry with exponential backoff |

### Backoff

- Initial delay: 500ms
- Maximum delay: 30 seconds
- Exponential: `500ms × 2^min(failures, 6)`

### Queue

- Maximum queue size: **500 messages**
- When the queue is full, the **oldest messages are dropped**
- Dropped messages are counted in the `dropped` stat

## HTTP Headers

Every HTTP request to the gateway includes:

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `X-Device-Id` | The device's ID |
| `X-Device-Token` | `GATEWAY_TOKEN`, falling back to `DEVICE_TOKEN` for compatibility |

## Monitoring

The gateway client's internal stats are available via `GET /control/status` (when `CONTROL_TOKEN` is set):

```json
{
  "gateway": {
    "registered": true,
    "queued": 3,
    "dropped": 0,
    "seq": 127
  }
}
```

| Stat | Description |
|---|---|
| `registered` | Whether the last registration succeeded |
| `queued` | Number of messages waiting for delivery |
| `dropped` | Number of messages dropped due to queue overflow |
| `seq` | Current sequence number (total messages created) |

## MQTT transport

When MQTT is selected, the runtime publishes to per-device `register`, `telemetry`, and `events` topics and subscribes to `commands` and `register/ack`. QoS 1 is at-least-once; gateway consumers must deduplicate by `(deviceId, bootId, seq)`. A broker PUBACK is not an application registration decision: the runtime waits for a matching registration acknowledgement before flushing queued messages. See the [MQTT 5 guide](../guides/mqtt.md) and the [MQTT gateway contract](https://github.com/aprountzos/fogsense-device-runtime/blob/feat/mqtt-device-transport/contract/mqtt-gateway.md).
