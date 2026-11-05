# MQTT gateway contract (MQTT 5.0)

This document defines the MQTT alternative to the HTTP gateway API. The device's local HTTP API (`/health`, `/info`, `/state`, `/commands`, and `/control/*`) remains unchanged.

## Connection

- Protocol: MQTT 5.0
- Supported broker URL schemes: `mqtt://` and `mqtts://`
- Default client ID: `fogsense-<DEVICE_ID>`
- Optional CONNECT username/password are configured independently from HTTP tokens.
- The device uses QoS 1 for publications and its command/registration-ack subscriptions.
- Messages are not retained.

## Topics

The topic prefix is `fogsense/v1/devices`.

| Topic | Publisher | Purpose |
| --- | --- | --- |
| `fogsense/v1/devices/{deviceId}/register` | Device | Registration and periodic heartbeat |
| `fogsense/v1/devices/{deviceId}/register/ack` | Gateway | Application-level registration result |
| `fogsense/v1/devices/{deviceId}/telemetry` | Device | Telemetry batches |
| `fogsense/v1/devices/{deviceId}/events` | Device | Device events |
| `fogsense/v1/devices/{deviceId}/commands` | Gateway | Commands for one device |
| `fogsense/v1/devices/{deviceId}/commands/ack` | Device | Command execution result |

## Registration

The device publishes the same registration object used by the HTTP gateway API to the `register` topic. A broker PUBACK confirms broker receipt only; it does not mean that the gateway application accepted the device. The device therefore waits for an application-level acknowledgement before it marks itself registered and begins flushing telemetry/events.

Example gateway acknowledgement:

```json
{
  "deviceId": "light-1",
  "bootId": "<current-boot-id>",
  "accepted": true
}
```

The device accepts the acknowledgement only when `deviceId` and `bootId` match its current runtime instance and `accepted` is `true`. The gateway should publish a negative acknowledgement with `accepted: false` when it rejects registration; the runtime will continue retrying registration.

## Telemetry and events

Telemetry and event payloads retain the existing HTTP envelope:

- `deviceId`: stable device identifier
- `bootId`: identifier generated for this runtime boot
- `seq`: monotonically increasing sequence number for this boot
- `ts`: UTC RFC3339Nano timestamp

QoS 1 is at-least-once delivery. The gateway must tolerate duplicates and deduplicate using `(deviceId, bootId, seq)`. Sequence values are shared across telemetry and events for a device boot.

## Commands

The gateway publishes the same JSON command payload accepted by the device's `POST /commands` endpoint:

```json
{
  "commandId": "cmd-123",
  "name": "SET_BRIGHTNESS",
  "params": { "level": 40 }
}
```

The device publishes the execution result to `commands/ack`, including `deviceId`, `commandId`, numeric `status`, and the existing response body. `commandId` is used by the device's existing command idempotency cache.

## Authentication and authorization

Use broker-side authentication and topic ACLs. Recommended per-device permissions:

- Publish only to that device's `register`, `telemetry`, `events`, and `commands/ack` topics.
- Subscribe only to that device's `commands` and `register/ack` topics.
- Deny wildcard subscriptions and access to other device IDs.
- Require TLS (`mqtts://`) outside isolated local development.

The local `docker-compose.mqtt.example.yml` playground deliberately enables anonymous broker access and is not a production security configuration. HTTP uses `GATEWAY_TOKEN` (sent as `X-Device-Token`) and `DEVICE_TOKEN` for the local device API; these are separate from MQTT broker credentials.
