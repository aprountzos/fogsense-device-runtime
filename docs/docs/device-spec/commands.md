---
id: commands
title: Commands
sidebar_label: Commands
sidebar_position: 4
---

# Commands

Commands are actions that can be sent to a device to change its state. Each command declares its parameters, validation rules, and the effects it has on state fields.

## Command Definition

```yaml
name: SET_BRIGHTNESS
params:
  level:
    type: number
    min: 0
    max: 100
effects:
  power: "ON"
  brightness: "$level"
userControllable: true
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | `string` | ✅ | Command name (`[A-Za-z][A-Za-z0-9_]{0,31}`) |
| `params` | `object` | ❌ | Parameter definitions (max 5 per command) |
| `effects` | `object` | ❌ | State changes to apply on execution |
| `userControllable` | `boolean` | ❌ | Whether end-users may invoke this command (default `false`) |

## Parameters

Each parameter has a type definition identical to [state fields](./state.md):

```yaml
params:
  target:
    type: number
    min: 10
    max: 35
  mode:
    type: enum
    values: ["COOL", "HEAT"]
  silent:
    type: boolean
```

**Validation rules:**
- All declared params are **required** when executing the command
- No extra (undeclared) params are allowed
- Values must fit the declared type and range

## Effects

Effects define how the command modifies state fields:

### Constant effects

Set a state field to a fixed value:

```yaml
effects:
  power: "ON"
```

The constant must fit the target state field's type/range.

### Parameter references

Set a state field to the value of a command parameter:

```yaml
effects:
  brightness: "$level"
```

The `$` prefix references a parameter by name. Type rules:
- The parameter type must match the state field type
- **Enum**: parameter values must be a **subset** of the state field's values
- **Number**: parameter `[min, max]` must lie **within** the state field's range

## Execution Flow

When a command is received:

```
Request → Validate → Check Idempotency → Execute → Emit Event (if changed)
```

### 1. Validation

The runtime checks:
- `commandId` is a string (1 to 100 characters)
- `name` matches a declared command
- All required params are present, no extras
- All param values fit their types/ranges

Invalid requests return `400` with a reason:

| Reason | When |
|---|---|
| `INVALID_REQUEST` | Missing/invalid `commandId` or `name` |
| `UNKNOWN_COMMAND` | Command name not in the spec |
| `INVALID_PARAMS` | Missing, extra, or out-of-range parameters |

### 2. Idempotency

Commands are **idempotent by `commandId`**:
- The runtime remembers the last 1,000 command IDs
- Replaying a `commandId` returns the cached result with `"duplicate": true`
- Effects and events are **not** applied again

:::caution
**Offline Exception**

Rejections due to `DEVICE_OFFLINE` are **not** cached. The same `commandId` can succeed on retry after the device recovers.
:::

### 3. Execution

If all checks pass:
1. Effects are applied to the state
2. The response includes the new state and `"changed": true/false`
3. If changed, a `STATE_CHANGED` event is sent to the gateway

## Request / Response Examples

### Successful command

**Request:**
```bash
curl -X POST http://device:8080/commands \
  -H 'Content-Type: application/json' \
  -d '{
    "commandId": "cmd-001",
    "name": "SET_BRIGHTNESS",
    "params": { "level": 75 }
  }'
```

**Response (200):**
```json
{
  "commandId": "cmd-001",
  "status": "EXECUTED",
  "state": { "power": "ON", "brightness": 75 },
  "changed": true
}
```

### Duplicate command

**Response (200):**
```json
{
  "commandId": "cmd-001",
  "status": "EXECUTED",
  "state": { "power": "ON", "brightness": 75 },
  "changed": true,
  "duplicate": true
}
```

### Invalid command

**Response (400):**
```json
{
  "commandId": "cmd-002",
  "status": "REJECTED",
  "reason": "INVALID_PARAMS",
  "message": "missing param 'level'"
}
```

## `userControllable` Flag

The `userControllable` boolean indicates whether the platform should allow end-users (as opposed to automated systems) to invoke this command.

:::important
The runtime itself **executes any declared command** regardless of this flag. Enforcing `userControllable` is the **platform's responsibility**.
:::

Example: A thermostat might let users `TURN_ON` and `SET_TEMPERATURE` but restrict `SET_MODE` to automated control:

```yaml
commands:
  - name: TURN_ON
    effects:
      power: "ON"
    userControllable: true
  - name: SET_MODE
    params:
      mode:
        type: enum
        values: ["COOL", "HEAT", "OFF"]
    effects:
      mode: "$mode"
    userControllable: false
```
