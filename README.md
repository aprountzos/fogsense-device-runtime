<p align="center">
  <img src="docs/static/img/logo.svg" alt="FogSense Logo" width="120">
</p>

# FogSense Device Runtime

One container = one simulated IoT device. The device is described **entirely by a YAML device spec**: there is no per-device-type code inside the runtime. The runtime is provided by the course; you build the platform that defines devices, starts them, and communicates with them.

Implemented in **Go**: a small static binary and a compact Docker image (check the exact size with `docker images`).

## Package Contents

| Path                                | Description                                                                                       |
| ----------------------------------- | ------------------------------------------------------------------------------------------------- |
| `main.go`, `pkg/`                   | The Go runtime implementation                                                                     |
| `Dockerfile`                        | Multi-stage Docker build producing a minimal static container                                     |
| `contract/device-spec.schema.yaml`  | Schema for the device spec (structure + semantic rules in `description`)                          |
| `contract/device-api.openapi.yaml`  | API exposed by each device (`/info`, `/state`, `/commands`, `/health`)                            |
| `contract/gateway-api.openapi.yaml` | API that **your Fog Gateway must implement** (`/devices/register`, `.../telemetry`, `.../events`) |
| `examples/*.yaml`                   | Sample specs (smart light, temperature sensor, thermostat, door lock)                             |
| `cmd/mock-gateway/`                 | Mock gateway for testing before you write your own                                                |
| `docs/`                             | Docusaurus documentation website                                                                  |

## At a Glance

The project follows a simple pattern:

1. One container = one device.
2. The device owns its local state and executes commands.
3. Each device registers with a configured gateway and sends telemetry and status events to it.
4. The gateway acts as the coordination and aggregation layer and can send commands back to the device.

### Command flow

```text
Gateway / platform
        │
        │  calls device API: POST /commands
        ▼
Device runtime
        │
        │  validates + applies command effects
        ▼
State changes
        │
        └──> emits STATE_CHANGED event back to the gateway
```

This is why command requests are sent to the device's own HTTP endpoint, not to the gateway URL.

## Documentation Site

```
cd docs
npm install
npm start
```

## Quick Test (without Docker)

Requires Go >= 1.22. You need **three terminals**: `go run` keeps running in the terminal where you start it.

**macOS / Linux (bash)**

```bash
# terminal 1: mock gateway (prints a line when it is listening)
go run ./cmd/mock-gateway --port 8081

# terminal 2: simulated smart light
DEVICE_ID=light-1 \
GATEWAY_URL=http://127.0.0.1:8081 \
ADVERTISE_URL=http://127.0.0.1:8080 \
DEVICE_SPEC_FILE=examples/smart-light.yaml \
go run .

# terminal 3: send a command
curl -X POST http://localhost:8080/commands -H 'Content-Type: application/json' \
  -d '{"commandId":"c1","name":"SET_BRIGHTNESS","params":{"level":40}}'
```

**Windows (PowerShell)**

```powershell
# terminal 1: mock gateway
go run ./cmd/mock-gateway --port 8081

# terminal 2: simulated smart light
$env:DEVICE_ID         = "light-1"
$env:GATEWAY_URL       = "http://127.0.0.1:8081"
$env:ADVERTISE_URL     = "http://127.0.0.1:8080"
$env:DEVICE_SPEC_FILE  = "examples/smart-light.yaml"
go run .

# terminal 3: send a command
Invoke-RestMethod -Method Post -Uri http://localhost:8080/commands `
  -ContentType "application/json" `
  -Body '{"commandId":"c1","name":"SET_BRIGHTNESS","params":{"level":40}}'
```

> In PowerShell, `curl` is an alias of `Invoke-WebRequest` and does not understand `-X`, `-H`, `-d`.
> Use `Invoke-RestMethod` as above, or call the real curl with `curl.exe`
> (quotes inside JSON must then be escaped: `-d "{\"commandId\":\"c1\",...}"`).
>
> Useful examples:
>
> ```powershell
> Invoke-RestMethod -Method Get -Uri http://localhost:8081/debug/devices
> Invoke-RestMethod -Method Post -Uri http://localhost:8080/commands `
>   -ContentType "application/json" `
>   -Body '{"commandId":"light-1-on","name":"TURN_ON"}'
> ```

## Running commands against multiple devices

Each device exposes its own HTTP API on its configured `PORT` (default `8080`). If you start more than one device, send commands to the device's own URL, not the gateway URL.

```bash
# device 1
DEVICE_ID=light-1 \
GATEWAY_URL=http://127.0.0.1:8081 \
ADVERTISE_URL=http://127.0.0.1:8080 \
PORT=8080 \
DEVICE_SPEC_FILE=examples/smart-light.yaml \
go run .

# device 2
DEVICE_ID=light-2 \
GATEWAY_URL=http://127.0.0.1:8081 \
ADVERTISE_URL=http://127.0.0.1:8082 \
PORT=8082 \
DEVICE_SPEC_FILE=examples/smart-light.yaml \
go run .
```

```bash
curl -X POST http://localhost:8080/commands \
  -H 'Content-Type: application/json' \
  -d '{"commandId":"light-1-on","name":"TURN_ON"}'

curl -X POST http://localhost:8082/commands \
  -H 'Content-Type: application/json' \
  -d '{"commandId":"light-2-on","name":"TURN_ON"}'
```

If `DEVICE_TOKEN` is enabled, add `-H 'X-Device-Token: <token>'` to each request.

## With Docker

**bash**

```bash
docker build -t fogsense-device-runtime .

docker run --rm --name light-1 --network <your-gateway-network> \
  -e DEVICE_ID=light-1 \
  -e GATEWAY_URL=http://fog-gateway:8081 \
  -e DEVICE_TOKEN=<device-token> \
  -e DEVICE_SPEC_B64="$(base64 -w0 examples/smart-light.yaml)" \
  fogsense-device-runtime
```

> On macOS, `base64` does not support `-w0`; use `base64 -i file | tr -d '\n'` instead.

**PowerShell**

```powershell
docker build -t fogsense-device-runtime .

$spec = [Convert]::ToBase64String([IO.File]::ReadAllBytes("examples/smart-light.yaml"))

docker run --rm --name light-1 --network <your-gateway-network> `
  -e DEVICE_ID=light-1 `
  -e GATEWAY_URL=http://fog-gateway:8081 `
  -e DEVICE_TOKEN=<device-token> `
  -e DEVICE_SPEC_B64=$spec `
  fogsense-device-runtime
```

`<your-gateway-network>` is the Docker network your Fog Gateway container is attached to, and the
name in `GATEWAY_URL` must be the gateway's service/container name on that network.
When using the course-published image, `docker build` is not needed.

`docker-compose.example.yml` starts a mock gateway and two devices:

```
docker compose -f docker-compose.example.yml up --build
```

## Configuration (Environment Variables)

| Variable                                               | Required     | Description                                                                                        |
| ------------------------------------------------------ | ------------ | -------------------------------------------------------------------------------------------------- |
| `DEVICE_ID`                                            | yes          | Unique id, `[A-Za-z0-9_.-]{1,64}`                                                                  |
| `GATEWAY_URL`                                          | yes          | Base URL of the Fog Gateway, e.g. `http://fog-gateway:8081`                                        |
| `DEVICE_SPEC_B64` / `DEVICE_SPEC` / `DEVICE_SPEC_FILE` | one of three | The spec: base64 YAML / YAML string / file path (default `/config/spec.yaml`)                      |
| `DEVICE_TOKEN`                                         | no           | Shared secret. The device sends it as `X-Device-Token` and requires it from anyone calling its API |
| `ADVERTISE_URL`                                        | no           | The URL the gateway uses to reach this device (default `http://<hostname>:<PORT>`)                 |
| `PORT`                                                 | no           | Default `8080`                                                                                     |
| `REGISTER_INTERVAL_SEC`                                | no           | Heartbeat registration period, default `30`                                                        |
| `TIME_SCALE`                                           | no           | Device time speed-up factor (for testing), default `1`                                             |
| `SEED`                                                 | no           | Integer for reproducible data generation                                                           |

If the spec is invalid, the container **does not start**: it prints all errors (with paths) as JSON lines and exits with code `2`.

## Device Behavior

**Telemetry.** Each metric produces a value every `intervalSec` seconds: a random walk within `[min, max]` plus Gaussian noise (`noise`); with probability `anomalyRate` the value goes **out of range**. If the metric has `follows`, it gradually moves toward the value of a numeric state field when the `when` conditions hold (e.g. a thermostat cooling toward its target).

**State and Commands.** A command validates its parameters, applies its `effects`, and returns the new state. Commands are **idempotent by `commandId`**. If the state changed, the device also sends a `STATE_CHANGED` event to the gateway.

**OFFLINE.** With probability `offlineRate` per second (or via an instructor-injected fault) the device goes offline for a few seconds. It first sends `STATUS_CHANGED(OFFLINE)` ("last will") and then **goes silent**: no telemetry, no heartbeat, and commands are rejected with `503 DEVICE_OFFLINE`.

**Reliability toward the gateway.** Every message carries `(bootId, seq)` and is delivered **at least once**, in order. If the gateway does not respond, the device keeps up to 500 messages in memory and retries with backoff. The full rules (`2xx`, `404`, `5xx`, other `4xx`) are documented in `contract/gateway-api.openapi.yaml`.

All durations are in **device time** (= real time, unless `TIME_SCALE` is set).

## Spec Limits

Up to 8 metrics, 8 state fields, 10 commands, 5 parameters per command, 16 values per enum. Names must match `[A-Za-z][A-Za-z0-9_]{0,31}`. The remaining semantic rules are described in the `description` field of `contract/device-spec.schema.yaml`. **Your Registry must validate them before starting any container**, so that the developer sees clear errors in the UI.

## Tests

```
go test -v ./...
```

**Contract conformance check.** The mock gateway can verify a running device against the contract
(`--check`): health, info, state, commands, idempotency, error handling, tokens, fault injection.
It prints PASS/FAIL per check and exits with code 0 only if everything passed. Use two terminals.

bash:

```bash
# terminal 1
go run ./cmd/mock-gateway --port 8081 --device-token t --control-token c --check

# terminal 2
DEVICE_ID=d1 GATEWAY_URL=http://127.0.0.1:8081 DEVICE_TOKEN=t CONTROL_TOKEN=c \
  ADVERTISE_URL=http://127.0.0.1:8080 DEVICE_SPEC_FILE=examples/thermostat.yaml go run .
```

PowerShell:

```powershell
# terminal 1
go run ./cmd/mock-gateway --port 8081 --device-token t --control-token c --check

# terminal 2
$env:DEVICE_ID="d1"; $env:GATEWAY_URL="http://127.0.0.1:8081"; $env:DEVICE_TOKEN="t"
$env:CONTROL_TOKEN="c"; $env:ADVERTISE_URL="http://127.0.0.1:8080"
$env:DEVICE_SPEC_FILE="examples/thermostat.yaml"
go run .
```

The number of checks depends on the spec (a sensor without commands has fewer checks than a thermostat).

## Troubleshooting

**`Unable to connect to the remote server` / `connection refused` on `localhost:8081`**
means that *nothing is listening on that port*: it is not a problem of the request itself.

1. `localhost:8081` is the **mock gateway**, not the device (the device listens on `8080`).
   Is the mock gateway running, in its own terminal, and did it print that it is listening?
   `go run` stops when you close or interrupt that terminal.
2. Check what listens on the port. Windows: `netstat -ano | findstr :8081`.
   macOS/Linux: `lsof -i :8081` or `ss -ltn | grep 8081`.
3. If you used Docker Compose: `docker compose -f docker-compose.example.yml ps` and
   `docker compose -f docker-compose.example.yml logs mock-gateway`. The container must be `running`
   and show `0.0.0.0:8081->8081/tcp`.
4. Try `http://127.0.0.1:8081/...` instead of `localhost` (IPv4 vs IPv6 resolution on Windows).
5. The port may be taken by another program: start the gateway with another `--port` and change
   `GATEWAY_URL` accordingly.
6. Inside a container, `127.0.0.1` is the container itself. A device container reaches the gateway
   by its service/container name on the shared Docker network, or through `host.docker.internal`
   when the gateway runs on your host.

**The device starts but nothing reaches the gateway** — check that `GATEWAY_URL` is reachable *from
the device* (see point 6), that both sides use the same `DEVICE_TOKEN`, and that `ADVERTISE_URL`
is a URL the *gateway* can reach.

**The container exits immediately** — the spec is invalid. Run `docker logs <container>`: every error
is printed with its path, and the exit code is `2`.

## What It Is Not

It does not persist anything (each restart begins from the initial state), is not safe for public exposure (the token is a shared secret over plain HTTP), and `cmd/mock-gateway` **is not a reference implementation** of the Fog Gateway: it does not perform aggregation, buffering, or broker communication.

## For maintainers

Publishing, GitHub Pages, CI and Docker Hub instructions are in [`MAINTAINERS.md`](MAINTAINERS.md).

## License

UNLICENSED