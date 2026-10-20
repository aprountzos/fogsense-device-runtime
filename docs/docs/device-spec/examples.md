---
id: examples
title: Example Specs
sidebar_label: Examples
sidebar_position: 5
---

# Example Device Specs

The `examples/` directory contains four complete device specs. Each demonstrates different features of the spec format.

## Temperature Sensor

**File:** `examples/temp-sensor.yaml`

A telemetry-only device with two metrics and random offline behavior.

```yaml
type: temp-sensor
version: 1
description: Temperature and humidity sensor
offlineRate: 0.001
telemetry:
  - name: temperature
    unit: "°C"
    min: 18
    max: 28
    noise: 0.4
    anomalyRate: 0.03
    intervalSec: 5
  - name: humidity
    unit: "%"
    min: 30
    max: 65
    noise: 1.0
    anomalyRate: 0.02
    intervalSec: 10
```

**Key features:**
- Two metrics with different intervals (5s and 10s)
- Gaussian noise on readings
- 3% / 2% anomaly rates for out-of-range values
- 0.1% chance per second of going offline randomly

---

## Smart Light

**File:** `examples/smart-light.yaml`

A state-only device (no telemetry) with power control and brightness adjustment.

```yaml
type: smart-light
version: 1
description: Dimmable smart light bulb
state:
  - name: power
    type: enum
    values: ["ON", "OFF"]
    initial: "OFF"
  - name: brightness
    type: number
    min: 0
    max: 100
    initial: 0
commands:
  - name: TURN_ON
    effects:
      power: "ON"
    userControllable: true
  - name: TURN_OFF
    effects:
      power: "OFF"
      brightness: 0
    userControllable: true
  - name: SET_BRIGHTNESS
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

**Key features:**
- Mixed state types (enum + number)
- Command with constant effects (`TURN_OFF` sets both `power` and `brightness`)
- Parameter reference (`$level` maps param to state)
- All commands are `userControllable`

---

## Thermostat

**File:** `examples/thermostat.yaml`

A full-featured device with telemetry that **follows** a state field.

```yaml
type: thermostat
version: 1
description: "Thermostat: temperature follows the target when powered on"
state:
  - name: power
    type: enum
    values: ["ON", "OFF"]
    initial: "OFF"
  - name: mode
    type: enum
    values: ["COOL", "HEAT", "OFF"]
    initial: "OFF"
  - name: targetTemp
    type: number
    min: 10
    max: 35
    initial: 22
telemetry:
  - name: temperature
    unit: "°C"
    min: 18
    max: 28
    noise: 0.2
    anomalyRate: 0
    intervalSec: 5
    follows:
      state: targetTemp
      when:
        power: "ON"
      rate: 0.2
commands:
  - name: TURN_ON
    effects:
      power: "ON"
    userControllable: true
  - name: TURN_OFF
    effects:
      power: "OFF"
    userControllable: true
  - name: SET_MODE
    params:
      mode:
        type: enum
        values: ["COOL", "HEAT", "OFF"]
    effects:
      mode: "$mode"
    userControllable: false
  - name: SET_TEMPERATURE
    params:
      target:
        type: number
        min: 10
        max: 35
    effects:
      targetTemp: "$target"
    userControllable: true
```

**Key features:**
- `follows` links temperature metric to `targetTemp` state field
- Temperature converges toward target **only when** `power` is `ON`
- `SET_MODE` is **not** `userControllable` (platform-managed)
- Three state types in one device

---

## Door Lock

**File:** `examples/door-lock.yaml`

A state-only device with enum and boolean fields.

```yaml
type: door-lock
version: 1
description: Smart door lock
state:
  - name: position
    type: enum
    values: ["OPEN", "CLOSED"]
    initial: "CLOSED"
  - name: locked
    type: boolean
    initial: true
commands:
  - name: OPEN
    effects:
      position: "OPEN"
      locked: false
    userControllable: true
  - name: CLOSE
    effects:
      position: "CLOSED"
    userControllable: true
  - name: LOCK
    effects:
      position: "CLOSED"
      locked: true
    userControllable: false
  - name: UNLOCK
    effects:
      locked: false
    userControllable: false
```

**Key features:**
- Boolean state field (`locked`)
- Commands with multiple effects (`OPEN` changes both position and lock)
- `LOCK` / `UNLOCK` restricted to automated systems (not `userControllable`)

---

## Creating Your Own Spec

Use these examples as templates. The general process:

1. Decide what your device **measures** (telemetry) and what it **does** (state + commands)
2. Define the telemetry metrics with appropriate ranges and intervals
3. Define the state fields with types and initial values
4. Define commands with parameters, effects, and access control
5. Validate by starting the runtime: it will report all errors at startup
