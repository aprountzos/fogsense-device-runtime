---
id: overview
title: Device Spec Overview
sidebar_label: Overview
sidebar_position: 1
---

# Device Spec Overview

The device spec is a YAML document that **completely describes** a simulated IoT device. The runtime reads this spec at startup and uses it to generate telemetry, manage state, and handle commands: no per-device-type code is needed.

## Structure

A device spec has the following top-level fields:

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

The runtime validates the spec at startup using both:

1. **Structural validation**: against the schema at `contract/device-spec.schema.yaml`
2. **Semantic validation**: cross-field rules enforced by `pkg/spec/spec.go`

If validation fails, the container **refuses to start**, prints all errors as structured log lines, and exits with code `2`.

### Semantic Rules

These rules cannot be expressed in schema alone and are enforced in code:

1. Names must be unique within telemetry, within state, and within commands
2. `min < max` everywhere (metrics, numeric state fields, numeric params)
3. Every key in a command's `effects` must be an existing state field
4. An effect value is either:
   - A **constant** that fits the state field's type/range, or
   - A **`$param` reference** to one of the command's own params, where the param's type equals the state field's type (enum params: values must be a subset; number params: range must lie within)
5. `follows.state` must name an existing numeric state field
6. Every key in `follows.when` must be an existing state field with a valid value
7. Enum values must be non-empty strings that do **not** start with `$`
8. `initial` must fit the declared type/range
9. At least one of `telemetry` or `state` must be non-empty

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

## Minimal Examples

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
