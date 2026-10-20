---
id: getting-started
title: Getting Started
sidebar_label: Getting Started
sidebar_position: 2
---

# Getting Started

This guide walks you through running your first simulated IoT device in under 5 minutes.

## Prerequisites

- **Go >= 1.22** (to run the device runtime and mock gateway)
- **Docker** (optional, for containerized deployment)

The runtime has zero external dependencies.

## Step 1: Start the Mock Gateway

The mock gateway simulates the Fog Gateway that your device will communicate with. Open a terminal and run:

```bash
go run ./cmd/mock-gateway --port 8081
```

You should see:

```
[HH:MM:SS] FogSense mock gateway listening on :8081
```

## Step 2: Start a Device

Open a **second terminal** and start a smart light device:

```bash
DEVICE_ID=light-1 \
GATEWAY_URL=http://127.0.0.1:8081 \
ADVERTISE_URL=http://127.0.0.1:8080 \
DEVICE_SPEC_FILE=examples/smart-light.yaml \
go run .
```

:::tip Windows PowerShell
On Windows, set variables and run:

```powershell
$env:DEVICE_ID="light-1"
$env:GATEWAY_URL="http://127.0.0.1:8081"
$env:ADVERTISE_URL="http://127.0.0.1:8080"
$env:DEVICE_SPEC_FILE="examples/smart-light.yaml"
go run .
```
:::

The device will register with the gateway and start its simulation loop.

## Step 3: Interact with the Device

Open a **third terminal** and use `curl` to interact:

### Check device health

```bash
curl http://localhost:8080/health
```

```json
{
  "status": "UP",
  "deviceId": "light-1",
  "bootId": "...",
  "registered": true
}
```

### Get device info

```bash
curl http://localhost:8080/info
```

### Get current state

```bash
curl http://localhost:8080/state
```

### Send a command

```bash
curl -X POST http://localhost:8080/commands \
  -H 'Content-Type: application/json' \
  -d '{"commandId":"c1","name":"SET_BRIGHTNESS","params":{"level":75}}'
```

```json
{
  "commandId": "c1",
  "status": "EXECUTED",
  "state": { "power": "ON", "brightness": 75 },
  "changed": true
}
```

## Step 4: Try Different Devices

The `examples/` directory contains sample specs for several device types:

| File | Device Type | Features |
|---|---|---|
| `smart-light.yaml` | Smart Light | Power on/off, brightness control |
| `temp-sensor.yaml` | Temperature Sensor | Temperature + humidity telemetry, random offline |
| `thermostat.yaml` | Thermostat | Temperature follows target when powered on |
| `door-lock.yaml` | Door Lock | Open/close/lock/unlock with boolean state |

Try starting a thermostat:

```bash
DEVICE_ID=thermo-1 \
GATEWAY_URL=http://127.0.0.1:8081 \
ADVERTISE_URL=http://127.0.0.1:8082 \
PORT=8082 \
DEVICE_SPEC_FILE=examples/thermostat.yaml \
go run .
```

## What's Next?

- Learn how to [write your own device spec](./device-spec/overview.md)
- Understand the [runtime behavior](./behavior/simulation-loop.md) in detail
- Set up [Docker deployment](./guides/docker.md) for production-like environments
- Explore the full [Device API](./api/device-api.md) and [Gateway API](./api/gateway-api.md)
