# MQTT gateway contract (MQTT 5.0)

This document defines the MQTT alternative to the HTTP gateway API. The device's local HTTP API (`/health`, `/info`, `/state`, `/commands`, and `/control/*`) remains unchanged.

## Connection

- Protocol: MQTT 5.0
- Supported broker URL schemes: `mqtt://` and `mqtts://`
- Default client ID: `fogsense-<DEVICE_ID>`
- Optional CONNECT username/password are configured independently from HTTP tokens.
- The device uses QoS 1 for publications and its command, registration-ack and delivery-ack subscriptions. The device also keeps a disk-backed outbox by default.
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
| `fogsense/v1/devices/{deviceId}/delivery/ack` | Gateway | Application-level acceptance of an outbound message |

## Registration

The device publishes the same registration object used by the HTTP gateway API to the `register` topic. A broker PUBACK confirms broker receipt only; it does not mean that the gateway application accepted the device. The device therefore waits for an application-level acknowledgement before it marks itself registered and begins flushing telemetry/events. This applies to periodic registration/heartbeat messages as well as initial registration and reconnects; queue delivery pauses until the matching acknowledgement arrives.

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
- `seq`: monotonically increasing sequence number persisted across restarts
- `ts`: UTC RFC3339Nano timestamp
- `messageId`: stable UUID generated once when enqueued and preserved across retries and process restarts

QoS 1 is at-least-once broker delivery, not gateway application acceptance. The device retains each message in its persistent outbox until it receives a matching `delivery/ack` with `accepted: true`. The gateway must tolerate duplicates and deduplicate using `(deviceId, messageId)`. `bootId` and `seq` remain diagnostic and ordering fields; the device rebinds recovered messages to its current boot while preserving their original messageId and sequence.

## Commands

The gateway publishes the same JSON command payload accepted by the device's `POST /commands` endpoint:

```json
{
  "commandId": "cmd-123",
  "name": "SET_BRIGHTNESS",
  "params": { "level": 40 }
}
```

The device queues the execution result for publication to `commands/ack`, including `deviceId`, `bootId`, `seq`, `ts`, `commandId`, numeric `status`, and the existing response body. Command acknowledgements use the same bounded persistent outbox and application-acknowledgement path as telemetry/events, so they survive a device process restart when the outbox volume is retained. `commandId` is used by the device's existing command idempotency cache; consumers must still tolerate QoS 1 duplicates.

## Authentication and authorization

Use broker-side authentication and topic ACLs. The secure local profile is provided by `docker-compose.mqtt.secure.example.yml`, `examples/mosquitto-secure.conf` and `examples/mosquitto-acl.example`. Configure `MQTT_CA_FILE` when the broker certificate is issued by a private CA; optional `MQTT_CLIENT_CERT_FILE` and `MQTT_CLIENT_KEY_FILE` enable mutual TLS. Recommended per-device permissions:

- Publish only to that device's `register`, `telemetry`, `events`, and `commands/ack` topics.
- Subscribe only to that device's `commands`, `register/ack`, and `delivery/ack` topics.
- Deny wildcard subscriptions and access to other device IDs.
- Require TLS (`mqtts://`) outside isolated local development.

The local `docker-compose.mqtt.example.yml` playground deliberately enables anonymous broker access and is not a production security configuration. MQTT over TLS requires username/password or a client certificate. Configure `MQTT_CA_FILE` for a private CA; optional `MQTT_CLIENT_CERT_FILE` and `MQTT_CLIENT_KEY_FILE` enable mutual TLS. The shared outbox defaults to `data/outbox-<DEVICE_ID>.json` and can be overridden with `OUTBOX_PATH`. It is atomically replaced and uses restrictive file permissions. Persist its containing directory/volume. The secure compose profile mounts named volumes for this purpose. HTTP uses `GATEWAY_TOKEN` (sent as `X-Device-Token`) and `DEVICE_TOKEN` for the local device API; these are separate from MQTT broker credentials.
