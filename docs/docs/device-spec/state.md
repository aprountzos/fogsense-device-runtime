---
id: state
title: State Fields
sidebar_label: State
sidebar_position: 3
---

# State Fields

State fields represent the current configuration or condition of a device. They can be modified by [commands](./commands.md) and are reported to the gateway when they change.

## State Field Definition

```yaml
name: power
type: enum
values: ["ON", "OFF"]
initial: "OFF"
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | `string` | ✅ | Unique field name (`[A-Za-z][A-Za-z0-9_]{0,31}`) |
| `type` | `string` | ✅ | One of `"enum"`, `"number"`, or `"boolean"` |
| `values` | `string[]` | ✅ (enum) | Allowed values for enum type (1 to 16 items) |
| `min` | `number` | ✅ (number) | Minimum allowed value |
| `max` | `number` | ✅ (number) | Maximum allowed value (`min < max` required) |
| `initial` | `any` | ❌ | Initial value at boot (see defaults below) |

## Supported Types

### Enum

An enumerated set of string values:

```yaml
name: mode
type: enum
values: ["COOL", "HEAT", "OFF"]
initial: "OFF"
```

- Up to 16 values
- Values must be non-empty strings
- Values must **not** start with `$` (reserved for parameter references)
- Default `initial`: first value in the array

### Number

A numeric value within a range:

```yaml
name: brightness
type: number
min: 0
max: 100
initial: 0
```

- Both `min` and `max` are required
- `min` must be strictly less than `max`
- Default `initial`: `min`

### Boolean

A true/false value:

```yaml
name: locked
type: boolean
initial: true
```

- Default `initial`: `false`

## State Events

When a command changes the device state, the runtime:

1. Applies the command's effects
2. Compares the new state with the previous state
3. If the state **changed**, emits a `STATE_CHANGED` event to the gateway:

```json
{
  "deviceId": "light-1",
  "bootId": "abc-123",
  "seq": 15,
  "ts": "2025-01-15T10:30:00.000Z",
  "eventId": "evt-uuid",
  "type": "STATE_CHANGED",
  "state": { "power": "ON", "brightness": 75 },
  "cause": { "commandId": "c1" }
}
```

If the command does **not** change any field (e.g., turning off a device that's already off), no event is emitted and the response includes `"changed": false`.

## Viewing State

The device API exposes the current state via `GET /state`:

```json
{
  "deviceId": "light-1",
  "bootId": "abc-123",
  "status": "ONLINE",
  "state": { "power": "ON", "brightness": 75 },
  "lastTelemetry": {
    "temperature": { "value": 22.3, "ts": "2025-01-15T10:30:00.000Z" }
  },
  "uptimeSec": 120
}
```

:::warning No Persistence
State is **not persisted** across restarts. Each boot starts from the `initial` values defined in the spec.
:::
