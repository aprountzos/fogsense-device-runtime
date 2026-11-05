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
| `mqtt-sensor` | Example temperature-sensor runtime publishing telemetry |

Follow the logs:

```bash
docker compose -f docker-compose.mqtt.example.yml logs -f mqtt-light mqtt-gateway
```

The example broker permits anonymous connections for local development only. Do not expose it to an untrusted network. For a TLS/authenticated local profile, see `docker-compose.mqtt.secure.example.yml` and the secure broker examples below.

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

The full topic and payload contract is in [MQTT Gateway Contract](https://github.com/aprountzos/fogsense-device-runtime/blob/main/contract/mqtt-gateway.md).

## Registration and delivery

A successful MQTT publish acknowledgement (PUBACK) confirms that the broker accepted a publication. It does **not** prove that the gateway application accepted the device registration.

FogSense therefore waits for a registration acknowledgement on the device's `register/ack` topic. The acknowledgement must contain the same `deviceId` and current `bootId`, with `accepted: true`. This also applies to periodic registration/heartbeat messages and reconnects. Queue delivery pauses until the matching acknowledgement is received.

Telemetry and events retain the existing envelope:

```json
{
  "deviceId": "light-1",
  "bootId": "boot-id-for-this-process",
  "seq": 42,
  "ts": "2026-11-05T12:00:00.000Z"
}
```

MQTT publications use QoS 1 (at-least-once broker delivery). The device does not remove an outbox message on PUBACK: it waits for the gateway application to publish a matching `delivery/ack` message containing `deviceId`, `messageId` and `accepted: true`. Consumers deduplicate using `(deviceId, messageId)`; `bootId` and `seq` are retained for diagnostics and ordering.

The outbound outbox is shared by HTTP and MQTT. It is disk-backed by default at `data/outbox-<DEVICE_ID>.json` (override with `OUTBOX_PATH`) and atomically saves enqueue, acknowledgement and sequence state. Each message has a stable `messageId`; recovered messages keep that ID and sequence while being associated with the current device boot. The outbox is bounded to 500 messages. When full, new messages are rejected and the dropped counter is incremented; messages are not silently evicted. Persist the containing directory or Docker volume.

## Secure MQTT playground

Prepare a private `secrets/mosquitto` directory (do not commit it). These OpenSSL commands create a local CA and a broker certificate with the required `DNS:mqtt-broker` subject alternative name:

```powershell
New-Item -ItemType Directory -Force secrets/mosquitto/certs | Out-Null
openssl req -x509 -newkey rsa:4096 -nodes -keyout secrets/mosquitto/certs/ca.key -out secrets/mosquitto/certs/ca.crt -days 3650 -subj "/CN=FogSense Local CA"
openssl req -newkey rsa:2048 -nodes -keyout secrets/mosquitto/certs/server.key -out secrets/mosquitto/certs/server.csr -subj "/CN=mqtt-broker"
@"
subjectAltName=DNS:mqtt-broker
extendedKeyUsage=serverAuth
"@ | Set-Content secrets/mosquitto/certs/server.ext
openssl x509 -req -in secrets/mosquitto/certs/server.csr -CA secrets/mosquitto/certs/ca.crt -CAkey secrets/mosquitto/certs/ca.key -CAcreateserial -out secrets/mosquitto/certs/server.crt -days 825 -sha256 -extfile secrets/mosquitto/certs/server.ext

# Create the password file interactively using mosquitto_passwd.
mosquitto_passwd -c secrets/mosquitto/passwd mqtt-light-1
mosquitto_passwd secrets/mosquitto/passwd mqtt-sensor-1
mosquitto_passwd secrets/mosquitto/passwd fogsense-mock-gateway
Copy-Item examples/mosquitto-acl.example secrets/mosquitto/acl
```

Set the three password variables in PowerShell, then start the profile:

```powershell
$env:MQTT_GATEWAY_PASSWORD = "..."
$env:MQTT_LIGHT_PASSWORD = "..."
$env:MQTT_SENSOR_PASSWORD = "..."
docker compose -f docker-compose.mqtt.secure.example.yml up --build
```

The usernames must match the ACL entries. The example uses TLS plus per-device username/password authentication. For mutual TLS, set `require_certificate true` in the Mosquitto configuration and provide client certificates with `MQTT_CLIENT_CERT_FILE` and `MQTT_CLIENT_KEY_FILE`.

## Send a command

Publish a command to the device's command topic:

```bash
docker compose -f docker-compose.mqtt.example.yml exec mqtt-broker \
  mosquitto_pub -h localhost \
  -t fogsense/v1/devices/mqtt-light-1/commands \
  -m '{"commandId":"mqtt-demo-1","name":"TURN_ON"}'
```

The runtime executes the command using the same device command engine as its HTTP API and queues the result for `commands/ack`. Command acknowledgements use the same bounded persistent outbox and application-acknowledgement path as telemetry and events, and survive a device process restart when the outbox volume is retained.

The local mock MQTT gateway logs received messages, persists accepted message IDs for deduplication, and acknowledges registration and message acceptance. It is a development tool, not a production gateway or transactional message processor.

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
