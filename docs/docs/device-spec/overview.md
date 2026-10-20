---
id: overview
title: Device Spec Overview
sidebar_label: Overview
sidebar_position: 1
---

# Device Spec Overview

A device spec is the complete definition of a simulated device. It tells the runtime which telemetry to emit, which state to maintain, and which commands are valid.

## At a glance

A spec is just a YAML document with a small set of top-level fields:

```yaml
type: smart-light
version: 1
description: A dimmable smart light bulb
offlineRate: 0.001
telemetry: [...]
state: [...]
commands: [...]
```

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | `string` | ✅ | Device type identifier. Must match `^[a-z0-9][a-z0-9-]{1,40}$` |
| `version` | `integer` | ✅ | Spec version, must be ≥ 1 |
| `description` | `string` | ❌ | Human-readable description |
| `offlineRate` | `number` | ❌ | Probability per second of going offline randomly (0 to 1, default `0`) |
| `telemetry` | `array` | ❌* | List of [telemetry metrics](./telemetry.md) |
| `state` | `array` | ❌* | List of [state fields](./state.md) |
| `commands` | `array` | ❌ | List of [commands](./commands.md) |

:::note
At least one of `telemetry` or `state` must be non-empty. A device with neither telemetry nor state is invalid.
:::

## Validation

The runtime validates each spec at startup in two passes:

1. **Schema validation** against the YAML contract
2. **Semantic validation** for cross-field rules enforced in code

If validation fails, the runtime exits immediately and prints structured errors. The container does not start in a partially valid state.

### Key semantic rules

These are the most important invariants enforced by the runtime:

- metric/state/command names must be unique within their own scope
- numeric ranges must satisfy `min < max`
- every command `effects` entry must target an existing state field
- effect values must be either a constant or a valid `$param` reference
- `follows.state` must reference an existing numeric field
- every key in `follows.when` must match an existing state field
- enum values must be non-empty strings and cannot start with `$`
- `initial` values must match the declared type and range
- at least one of `telemetry` or `state` must be present

## Limits

| Resource | Maximum |
|---|---|
| Telemetry metrics | 8 |
| State fields | 8 |
| Commands | 10 |
| Parameters per command | 5 |
| Enum values | 16 |
| Name length | 32 characters |

Names must match the pattern `[A-Za-z][A-Za-z0-9_]{0,31}`.

## Minimal examples

### Telemetry-only device (sensor)

```yaml
type: temp-sensor
version: 1
telemetry:
  - name: temperature
    unit: "°C"
    min: 18
    max: 28
```

### State-only device (actuator)

```yaml
type: door-lock
version: 1
state:
  - name: locked
    type: boolean
    initial: true
commands:
  - name: UNLOCK
    effects:
      locked: false
  - name: LOCK
    effects:
      locked: true
```

### Full device (sensor + actuator)

See the [Examples](./examples.md) page for complete, real-world specs.
