---
id: offline-mode
title: Offline Mode
sidebar_label: Offline Mode
sidebar_position: 2
---

# Offline Mode

Devices can go offline either randomly (based on `offlineRate`) or through instructor-injected faults. Offline behavior is a key part of the simulation, testing the platform's ability to handle device disconnections.

## How Offline Works

When a device goes **OFFLINE**:

1. It sends a `STATUS_CHANGED(OFFLINE)` event to the gateway (a "last will" message)
2. **All telemetry stops**: no readings are generated
3. **Heartbeat stops**: no registration/heartbeat messages are sent
4. **Commands are rejected** with `503 DEVICE_OFFLINE`
5. The device remains silent for the offline duration

When the device **recovers**:

1. It sends a `STATUS_CHANGED(ONLINE)` event
2. All metric timers reset: telemetry resumes immediately
3. It re-registers with the gateway right away
4. Normal operation continues

## Random Offline

If the spec defines `offlineRate > 0`, the device randomly goes offline:

```json
{
  "offlineRate": 0.001
}
```

- The probability is evaluated **per second of device time**
- Offline duration: random between **5 and 20 seconds** (device time)
- Reason: `"RANDOM"`

The actual probability per tick is computed as:
```
p = 1 - (1 - offlineRate)^dt
```

This ensures the probability is consistent regardless of tick frequency.

## Fault-Injected Offline

Instructors can force a device offline via the [Control API](../api/device-api.md#control-api-instructor-only):

```bash
curl -X POST http://device:8080/control/fault \
  -H 'X-Control-Token: <token>' \
  -H 'Content-Type: application/json' \
  -d '{"action":"OFFLINE","durationSec":120}'
```

- Default duration: 60 seconds (device time)
- Maximum duration: 86,400 seconds
- Reason: `"FAULT_INJECTED"`

To recover immediately:

```bash
curl -X POST http://device:8080/control/fault \
  -H 'X-Control-Token: <token>' \
  -H 'Content-Type: application/json' \
  -d '{"action":"CLEAR"}'
```

## Status Events

```json
{
  "type": "STATUS_CHANGED",
  "status": "OFFLINE",
  "reason": "RANDOM"
}
```

```json
{
  "type": "STATUS_CHANGED",
  "status": "ONLINE",
  "reason": "RECOVERED"
}
```

Possible `reason` values:

| Reason | Trigger |
|---|---|
| `RANDOM` | Random offline based on `offlineRate` |
| `FAULT_INJECTED` | Instructor used the Control API |
| `RECOVERED` | Duration expired, device came back |
| `FAULT_CLEARED` | Instructor used `CLEAR` action |

## Command Behavior While Offline

Commands sent to an offline device return:

```json
{
  "commandId": "...",
  "status": "REJECTED",
  "reason": "DEVICE_OFFLINE"
}
```

HTTP status: **503**

:::important
Offline rejections are **not cached** for idempotency. If you retry the same `commandId` after the device recovers, it will be executed normally.
:::

## Other Fault Types

Besides offline, the Control API supports two other fault types:

### Spike

Forces anomalous (out-of-range) values for a specified duration:

```bash
curl -X POST http://device:8080/control/fault \
  -H 'X-Control-Token: <token>' \
  -H 'Content-Type: application/json' \
  -d '{"action":"SPIKE","metric":"temperature","durationSec":30}'
```

- Omit `metric` to spike **all** metrics
- Default duration: 30 seconds

### Flood

Increases telemetry emission rate:

```bash
curl -X POST http://device:8080/control/fault \
  -H 'X-Control-Token: <token>' \
  -H 'Content-Type: application/json' \
  -d '{"action":"FLOOD","multiplier":10,"durationSec":30}'
```

- Multiplier range: 2 to 100
- Default multiplier: 10
- Default duration: 30 seconds
