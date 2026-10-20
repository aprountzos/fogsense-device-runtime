---
id: configuration
title: Configuration Reference
sidebar_label: Configuration
sidebar_position: 4
---

# Configuration Reference

The device runtime is configured entirely through **environment variables**. No configuration files are needed beyond the device spec itself.

## Required Variables

| Variable | Format | Description |
|---|---|---|
| `DEVICE_ID` | `[A-Za-z0-9_.-]{1,64}` | Unique identifier for this device instance |
| `GATEWAY_URL` | `http://...` or `https://...` | Base URL of the Fog Gateway |

## Device Spec (One Required)

The device spec can be provided in three ways. The runtime checks them in this priority order:

| Variable | Format | Description |
|---|---|---|
| `DEVICE_SPEC_B64` | Base64 string | Spec YAML encoded as base64 (most portable) |
| `DEVICE_SPEC` | YAML string | Spec as a raw YAML string |
| `DEVICE_SPEC_FILE` | File path | Path to a YAML file (default: `/config/spec.yaml`) |

### Examples

**Base64 (recommended for Docker):**
```bash
DEVICE_SPEC_B64="$(base64 -w0 examples/smart-light.yaml)"
```

**YAML string:**
```bash
DEVICE_SPEC='type: temp-sensor
version: 1
telemetry:
  - name: t
    min: 0
    max: 100'
```

**File path:**
```bash
DEVICE_SPEC_FILE=examples/smart-light.yaml
```

## Optional Variables

| Variable | Default | Description |
|---|---|---|
| `DEVICE_TOKEN` | _(none)_ | Shared secret for device API authentication. Sent as `X-Device-Token` header and required from API callers |
| `ADVERTISE_URL` | `http://<hostname>:<PORT>` | URL the gateway uses to reach this device. Override when the hostname isn't routable (e.g., Docker) |
| `PORT` | `8080` | HTTP port for the device API |
| `REGISTER_INTERVAL_SEC` | `30` | Seconds between heartbeat registrations |
| `TIME_SCALE` | `1` | Device time multiplier (0 to 1000]. `10` means device time runs 10x faster |
| `SEED` | _(random)_ | Integer seed for the PRNG. Same seed = same telemetry values |
| `CONTROL_TOKEN` | _(none)_ | Enables the instructor Control API (`/control/*`). Requires `X-Control-Token` header |

## Startup Validation

At startup, the runtime validates:

1. `DEVICE_ID`: must be present and match `[A-Za-z0-9_.-]{1,64}`
2. `GATEWAY_URL`: must be present and start with `http://` or `https://`
3. `TIME_SCALE`: must be a number in `(0, 1000]`
4. Device spec: must be valid YAML that passes all structural and semantic validation

If any validation fails, the runtime:
- Logs the error(s) as structured log lines
- Exits with code **2**

## Example Configurations

### Minimal (local testing)

```bash
DEVICE_ID=sensor-1
GATEWAY_URL=http://127.0.0.1:8081
DEVICE_SPEC_FILE=examples/temp-sensor.yaml
```

### Docker with authentication

```bash
docker run --rm \
  -e DEVICE_ID=light-1 \
  -e GATEWAY_URL=http://fog-gateway:8081 \
  -e DEVICE_TOKEN=secret-token-123 \
  -e ADVERTISE_URL=http://light-1:8080 \
  -e DEVICE_SPEC_B64="$(base64 -w0 spec.yaml)" \
  fogsense-device-runtime
```

### Fast testing with reproducible data

```bash
DEVICE_ID=test-device
GATEWAY_URL=http://127.0.0.1:8081
DEVICE_SPEC_FILE=examples/thermostat.yaml
TIME_SCALE=10
SEED=42
```

### Instructor exam setup

```bash
docker run --rm \
  -e DEVICE_ID=exam-device-1 \
  -e GATEWAY_URL=http://student-gateway:8081 \
  -e DEVICE_TOKEN=exam-token \
  -e CONTROL_TOKEN=instructor-secret \
  -e ADVERTISE_URL=http://exam-device-1:8080 \
  -e DEVICE_SPEC_B64="$(base64 -w0 custom-spec.yaml)" \
  fogsense-device-runtime
```
