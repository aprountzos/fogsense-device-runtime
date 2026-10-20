<p align="center">
  <img src="docs/static/img/logo.svg" alt="FogSense Logo" width="130" height="130" />
</p>

# FogSense Device Runtime

One container = one simulated IoT device. The device is described **entirely by a YAML device spec**: there is no per-device-type code inside the runtime. The runtime is provided by the course; you build the platform that defines devices, starts them, and communicates with them.

Implemented in **Go** for high efficiency (< 15 MB RAM per container and a compact ~35 MB Docker image).

## Package Contents

| Path | Description |
|---|---|
| `main.go`, `pkg/` | High-efficiency Go runtime implementation |
| `Dockerfile` | Multi-stage Docker build producing a minimal static container |
| `contract/device-spec.schema.yaml` | Schema for the device spec (structure + semantic rules in `description`) |
| `contract/device-api.openapi.yaml` | API exposed by each device (`/info`, `/state`, `/commands`, `/health`) |
| `contract/gateway-api.openapi.yaml` | API that **your Fog Gateway must implement** (`/devices/register`, `.../telemetry`, `.../events`) |
| `examples/*.yaml` | Sample specs (smart light, temperature sensor, thermostat, door lock) |
| `cmd/mock-gateway/` | Mock gateway in Go for testing before you write your own |
| `docs/` | Complete Docusaurus documentation website |
| `.github/workflows/` | GitHub Actions for CI testing, Docker Hub publishing, and GitHub Pages |

## Documentation Site

A full documentation website powered by Docusaurus is included in the `docs/` directory:

- **Intro & Getting Started**: Fast start guide and architectural concepts
- **Device Spec Guide**: Metrics, state, commands, and example specifications
- **Device Behavior**: Simulation loops, fault injection, and gateway communication
- **API Reference**: Detailed OpenAPI contracts for Device API and Gateway API
- **Guides**: Docker usage, mock gateway testing, configuration, and unit testing

To run the documentation locally:

```bash
cd docs
npm install
npm start
```

## Quick Test (without Docker)

Requires Go >= 1.22.

```bash
# terminal 1: start mock gateway
go run ./cmd/mock-gateway --port 8081

# terminal 2: start simulated smart light
DEVICE_ID=light-1 \
GATEWAY_URL=http://127.0.0.1:8081 \
ADVERTISE_URL=http://127.0.0.1:8080 \
DEVICE_SPEC_FILE=examples/smart-light.yaml \
go run .

# terminal 3: send a command
curl -X POST http://localhost:8080/commands -H 'Content-Type: application/json' \
  -d '{"commandId":"c1","name":"SET_BRIGHTNESS","params":{"level":40}}'
```

## With Docker

```bash
docker build -t fogsense-device-runtime .

docker run --rm --name light-1 --network <your-gateway-network> \
  -e DEVICE_ID=light-1 \
  -e GATEWAY_URL=http://fog-gateway:8081 \
  -e DEVICE_TOKEN=<device-token> \
  -e DEVICE_SPEC_B64="$(base64 -w0 examples/smart-light.yaml)" \
  fogsense-device-runtime
```

The `docker-compose.example.yml` sets up a mock gateway and two devices.
When using the course-published image, `docker build` is not needed.

> On macOS, `base64` does not support `-w0`; use `base64 -i file | tr -d '\n'` instead.

## Configuration (Environment Variables)

| Variable | Required | Description |
|---|---|---|
| `DEVICE_ID` | yes | Unique id, `[A-Za-z0-9_.-]{1,64}` |
| `GATEWAY_URL` | yes | Base URL of the Fog Gateway, e.g. `http://fog-gateway:8081` |
| `DEVICE_SPEC_B64` / `DEVICE_SPEC` / `DEVICE_SPEC_FILE` | one of three | The spec: base64 YAML / YAML string / file path (default `/config/spec.yaml`) |
| `DEVICE_TOKEN` | no | Shared secret. The device sends it as `X-Device-Token` and requires it from anyone calling its API |
| `ADVERTISE_URL` | no | The URL the gateway uses to reach this device (default `http://<hostname>:<PORT>`) |
| `PORT` | no | Default `8080` |
| `REGISTER_INTERVAL_SEC` | no | Heartbeat registration period, default `30` |
| `TIME_SCALE` | no | Device time speed-up factor (for testing), default `1` |
| `SEED` | no | Integer for reproducible data generation |

If the spec is invalid, the container **does not start**: it prints all errors (with JSON paths) as JSON lines and exits with code `2`.

## Device Behavior

**Telemetry.** Each metric produces a value every `intervalSec` seconds: a random walk within `[min, max]` plus Gaussian noise (`noise`); with probability `anomalyRate` the value goes **out of range**. If the metric has `follows`, it gradually moves toward the value of a numeric state field when the `when` conditions hold (e.g. a thermostat cooling toward its target).

**State and Commands.** A command validates its parameters, applies its `effects`, and returns the new state. Commands are **idempotent by `commandId`**. If the state changed, the device also sends a `STATE_CHANGED` event to the gateway.

**OFFLINE.** With probability `offlineRate` per second (or via an instructor-injected fault) the device goes offline for a few seconds. It first sends `STATUS_CHANGED(OFFLINE)` ("last will") and then **goes silent**: no telemetry, no heartbeat, and commands are rejected with `503 DEVICE_OFFLINE`.

**Reliability toward the gateway.** Every message carries `(bootId, seq)` and is delivered **at least once**, in order. If the gateway does not respond, the device keeps up to 500 messages in memory and retries with backoff. The full rules (`2xx`, `404`, `5xx`, other `4xx`) are documented in `contract/gateway-api.openapi.yaml`.

All durations are in **device time** (= real time, unless `TIME_SCALE` is set).

## Spec Limits

Up to 8 metrics, 8 state fields, 10 commands, 5 parameters per command, 16 values per enum. Names must match `[A-Za-z][A-Za-z0-9_]{0,31}`. The remaining semantic rules are described in the `description` field of `contract/device-spec.schema.yaml`. **Your Registry must validate them before starting any container**, so that the developer sees clear errors in the UI.

## Tests

```bash
# 1. Run Go unit tests:
go test -v ./...

# 2. Full contract conformance check with mock gateway:
go run ./cmd/mock-gateway --port 8081 --device-token t --control-token c --check &
DEVICE_ID=d1 GATEWAY_URL=http://127.0.0.1:8081 DEVICE_TOKEN=t CONTROL_TOKEN=c \
  ADVERTISE_URL=http://127.0.0.1:8080 DEVICE_SPEC_FILE=examples/thermostat.yaml go run .
```

The `--check` flag runs 31 automated conformance checks (health, info, state, commands, idempotency, error handling, tokens, fault injection) and outputs PASS/FAIL.

## What It Is Not

It does not persist anything (each restart begins from the initial state), is not safe for public exposure (the token is a shared secret over plain HTTP), and `cmd/mock-gateway` **is not a reference implementation** of the Fog Gateway: it does not perform aggregation, buffering, or broker communication.

## License

UNLICENSED
