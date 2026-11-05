---
id: mqtt
title: MQTT 5 Transport
sidebar_label: MQTT
sidebar_position: 4
---

# MQTT 5 Transport

FogSense supports two independent ways for a device runtime to communicate with its gateway:

- **HTTP** — the default transport and the original gateway API contract.
- **MQTT 5.0** — an alternative broker-based transport for registration, telemetry, events and commands.

The device's own HTTP API remains available in both modes. MQTT does not replace the local `/health`, `/info`, `/state`, `/commands` or `/control/*` endpoints.

## When to choose MQTT

MQTT is useful when devices communicate over broker-based networks, when the gateway needs to distribute commands asynchronously, or when the infrastructure already provides an MQTT broker.

Use HTTP when you already have an HTTP gateway and prefer request/response delivery. You do not need an MQTT broker to run FogSense in its default mode.

## Start the MQTT playground

The repository provides a local Mosquitto broker, a mock MQTT gateway and a YAML-configured smart-light device.

From the repository root:

```bash
docker compose -f docker-compose.mqtt.example.yml up --build
```

This starts:

| Service | Purpose |
|---|---|
| `mqtt-broker` | Local Mosquitto broker on port `1883` |
| `mqtt-gateway` | Subscribes to device topics, logs messages and acknowledges registrations |
| `mqtt-light` | Example smart-light runtime using MQTT |

Follow the logs:

```bash
docker compose -f docker-compose.mqtt.example.yml logs -f mqtt-light mqtt-gateway
```

The example broker permits anonymous connections for local development only. Do not expose it to an untrusted network.

## Configure a device

Set `GATEWAY_TRANSPORT=mqtt` and provide the broker URL:

```bash
DEVICE_ID=light-1 \
GATEWAY_TRANSPORT=mqtt \
MQTT_BROKER_URL=mqtt://127.0.0.1:1883 \
ADVERTISE_URL=http://127.0.0.1:8080 \
DEVICE_SPEC_FILE=examples/smart-light.yaml \
go run .
```

PowerShell:

```powershell
$env:DEVICE_ID = "light-1"
$env:GATEWAY_TRANSPORT = "mqtt"
$env:MQTT_BROKER_URL = "mqtt://127.0.0.1:1883"
$env:ADVERTISE_URL = "http://127.0.0.1:8080"
$env:DEVICE_SPEC_FILE = "examples/smart-light.yaml"
go run .
```

The runtime creates the default client ID `fogsense-<DEVICE_ID>` when `MQTT_CLIENT_ID` is not set.

## Topics

The current topic namespace is `fogsense/v1/devices`.

| Direction | Topic | Purpose |
|---|---|---|
| Device → gateway | `fogsense/v1/devices/{deviceId}/register` | Registration and heartbeat |
| Gateway → device | `fogsense/v1/devices/{deviceId}/register/ack` | Application-level registration result |
| Device → gateway | `fogsense/v1/devices/{deviceId}/telemetry` | Telemetry batches |
| Device → gateway | `fogsense/v1/devices/{deviceId}/events` | State/status events |
| Gateway → device | `fogsense/v1/devices/{deviceId}/commands` | Device commands |
| Device → gateway | `fogsense/v1/devices/{deviceId}/commands/ack` | Command execution result |

The full topic and payload contract is in [MQTT Gateway Contract](../../../../contract/mqtt-gateway.md).

## Registration and delivery

A successful MQTT publish acknowledgement (PUBACK) confirms that the broker accepted a publication. It does **not** prove that the gateway application accepted the device registration.

FogSense therefore waits for a registration acknowledgement on the device's `register/ack` topic. The acknowledgement must contain the same `deviceId` and current `bootId`, with `accepted: true`. Only then does the runtime mark itself registered and flush queued telemetry and events.

Telemetry and events retain the existing envelope:

```json
{
  "deviceId": "light-1",
  "bootId": "boot-id-for-this-process",
  "seq": 42,
  "ts": "2026-11-05T12:00:00.000Z"
}
```

MQTT publications use QoS 1 (at-least-once). Consumers must tolerate duplicates and deduplicate using `(deviceId, bootId, seq)`. Sequence numbers are shared across telemetry and events for one device boot.

The outbound queue is shared by the HTTP and MQTT transports. MQTT publish failures leave messages queued for retry. The queue is in memory; it does not survive a device process restart.

## Send a command

Publish a command to the device's command topic:

```bash
docker compose -f docker-compose.mqtt.example.yml exec mqtt-broker \
  mosquitto_pub -h localhost \
  -t fogsense/v1/devices/mqtt-light-1/commands \
  -m '{"commandId":"mqtt-demo-1","name":"TURN_ON"}'
```

The runtime executes the command using the same device command engine as its HTTP API and publishes the result to `commands/ack`.

The local mock MQTT gateway currently logs received messages and acknowledges registrations. It is a development tool, not a production gateway or durable message processor.

## Authentication and TLS

MQTT credentials are independent from the device's local HTTP API token and the HTTP gateway token.

| Variable | Required | Description |
|---|---|---|
| `MQTT_BROKER_URL` | MQTT | Broker URL: `mqtt://host:1883` or `mqtts://host:8883` |
| `MQTT_CLIENT_ID` | No | Client identifier; defaults to `fogsense-<DEVICE_ID>` |
| `MQTT_USERNAME` | No | MQTT CONNECT username |
| `MQTT_PASSWORD` | No | MQTT CONNECT password; set together with username |

Example for a TLS-enabled broker:

```bash
GATEWAY_TRANSPORT=mqtt
MQTT_BROKER_URL=mqtts://broker.example.com:8883
MQTT_CLIENT_ID=fogsense-light-1
MQTT_USERNAME=light-1
MQTT_PASSWORD=<inject-from-secret-store>
```

For production deployments:

- Require TLS and broker-side authentication.
- Use unique credentials per device.
- Apply broker ACLs so each device can publish only to its own `register`, `telemetry`, `events` and `commands/ack` topics.
- Allow each device to subscribe only to its own `commands` and `register/ack` topics.
- Deny wildcard subscriptions and access to other device IDs.
- Store credentials in a secret manager; do not commit them to YAML files or source control.

## Keep the local HTTP API

Regardless of the gateway transport, the device API continues to listen on `PORT` (default `8080`). Use it for health checks, inspecting state and direct local command execution. If `DEVICE_TOKEN` is configured, API callers must supply `X-Device-Token`.

## Related documentation

- [Gateway Communication](../behavior/gateway-communication.md)
- [Configuration Reference](./configuration.md)
- [Running with Docker](./docker.md)
- [HTTP Gateway API](../api/gateway-api.md)
