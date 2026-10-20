---
id: docker
title: Running with Docker
sidebar_label: Docker
sidebar_position: 1
---

# Running with Docker

The FogSense Device Runtime is designed to run as a Docker container: one container per simulated device.

## Building the Image

```bash
docker build -t fogsense-device-runtime .
```

The Dockerfile uses a multi-stage Go build, producing a minimal static binary inside a slim Alpine container (under 18 MB total image size).

## Running a Single Device

```bash
docker run --rm --name light-1 \
  --network <your-gateway-network> \
  -e DEVICE_ID=light-1 \
  -e GATEWAY_URL=http://fog-gateway:8081 \
  -e DEVICE_TOKEN=<token> \
  -e DEVICE_SPEC_B64="$(base64 -w0 examples/smart-light.yaml)" \
  fogsense-device-runtime
```

:::tip macOS

On macOS, `base64` doesn't support `-w0`. Use:

```bash
base64 -i examples/smart-light.yaml | tr -d '\n'

### Passing the Device Spec

There are three ways to provide the device spec:

| Method | Environment Variable | Best For |
|---|---|---|
| Base64 encoded | `DEVICE_SPEC_B64` | Docker commands, CI/CD |
| YAML string | `DEVICE_SPEC` | Simple inline specs |
| File path | `DEVICE_SPEC_FILE` | Volume-mounted configs |

**Using a volume-mounted file:**
```bash
docker run --rm --name sensor-1 \
  -v $(pwd)/examples/temp-sensor.yaml:/config/spec.yaml:ro \
  -e DEVICE_ID=sensor-1 \
  -e GATEWAY_URL=http://fog-gateway:8081 \
  -e DEVICE_SPEC_FILE=/config/spec.yaml \
  fogsense-device-runtime
```

## Docker Compose

The repository includes `docker-compose.example.yml` that sets up a complete local environment:

```yaml
services:
  mock-gateway:
    build: .
    entrypoint: ["fogsense-mock-gateway"]
    command: ["--port", "8081", "--device-token", "demo-token"]
    ports:
      - "8081:8081"

  light-1:
    build: .
    depends_on: [mock-gateway]
    environment:
      DEVICE_ID: light-1
      GATEWAY_URL: http://mock-gateway:8081
      DEVICE_TOKEN: demo-token
      ADVERTISE_URL: http://light-1:8080
      DEVICE_SPEC_FILE: /config/spec.yaml
    volumes:
      - ./examples/smart-light.yaml:/config/spec.yaml:ro

  sensor-1:
    build: .
    depends_on: [mock-gateway]
    environment:
      DEVICE_ID: sensor-1
      GATEWAY_URL: http://mock-gateway:8081
      DEVICE_TOKEN: demo-token
      ADVERTISE_URL: http://sensor-1:8080
      DEVICE_SPEC_FILE: /config/spec.yaml
      TIME_SCALE: "2"
    volumes:
      - ./examples/temp-sensor.yaml:/config/spec.yaml:ro
```

**Start the playground:**
```bash
docker compose -f docker-compose.example.yml up --build
```

**Query the mock gateway:**
```bash
curl http://localhost:8081/debug/devices
```

## Health Check

The Docker image includes a built-in health check:

```dockerfile
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT}/health" >/dev/null || exit 1
```

This calls the device's `/health` endpoint every 15 seconds.

## Multi-Architecture Builds

For teams with mixed architectures (e.g., Apple Silicon + x86):

```bash
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 \
  -t <username>/fogsense-device-runtime:1.0.0 \
  -t <username>/fogsense-device-runtime:latest --push .
```

## Resource Usage

A single Go device container uses approximately **8-15 MB RAM** (RSS). On a standard small VM (2-4 GB RAM), you can run hundreds of devices concurrently without memory exhaustion.

The command deduplication cache holds the last 1,000 `commandId` entries, and the outbound message queue holds up to 500 messages.
