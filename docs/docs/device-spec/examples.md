---
id: examples
title: Example Specs
sidebar_label: Examples
sidebar_position: 5
---

# Example Device Specs

The `examples/` directory contains complete, production-ready device specs spanning consumer IoT, smart buildings, renewable energy, and industrial systems.

| Spec File | Device Type | Category | Highlights |
| :--- | :--- | :--- | :--- |
| [`temp-sensor.yaml`](#temperature-sensor) | `temp-sensor` | Environmental | Telemetry with noise and anomaly rates |
| [`smart-light.yaml`](#smart-light) | `smart-light` | Smart Home | State-only, parameterized brightness |
| [`thermostat.yaml`](#thermostat) | `thermostat` | HVAC | Telemetry follows state target when powered on |
| [`door-lock.yaml`](#door-lock) | `door-lock` | Security | Role-based commands and multi-effect transitions |
| [`ev-charger.yaml`](#ev-charging-station) | `ev-charger` | E-Mobility | AC charging, state machine, current throttling, emergency stop |
| [`solar-inverter.yaml`](#solar-inverter) | `solar-inverter` | Clean Energy | Multi-phase generation, grid frequency, curtailment control |
| [`vibration-sensor.yaml`](#machinery-vibration-sensor) | `vibration-sensor` | Industrial IIoT | High-frequency RMS vibration, acoustic emission, anomaly trigger |
| [`cold-chain-tracker.yaml`](#cold-chain-asset-tracker) | `cold-chain-tracker` | Logistics | Refrigerator cargo temperature, door state, battery drain |
| [`water-meter.yaml`](#smart-water-meter) | `water-meter` | Utilities | Flow rate, total volume counter, shutoff valve |
| [`smoke-alarm.yaml`](#smart-smoke-alarm) | `smoke-alarm` | Safety | Particle density, siren trigger, battery test |

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

## EV Charging Station

**File:** `examples/ev-charger.yaml`

A commercial electric vehicle charging station with active charging telemetry, dynamic current throttling, and safety interlocks.

```yaml
type: ev-charger
version: 1
description: Commercial electric vehicle AC charging station
telemetry:
  - name: activePowerKw
    unit: "kW"
    min: 0
    max: 22
    noise: 0.15
    anomalyRate: 0.01
    intervalSec: 5
    follows:
      state: chargingPower
      when:
        status: CHARGING
      rate: 0.5
  - name: totalEnergyKwh
    unit: "kWh"
    min: 0
    max: 10000
    noise: 0.05
    intervalSec: 10
  - name: cableTempC
    unit: "C"
    min: 15
    max: 85
    noise: 0.4
    intervalSec: 5
  - name: lineVoltageV
    unit: "V"
    min: 210
    max: 250
    noise: 0.8
    intervalSec: 5
state:
  - name: status
    type: enum
    values: [AVAILABLE, PREPARING, CHARGING, SUSPENDED, FAULTED]
    initial: AVAILABLE
  - name: chargingPower
    type: number
    min: 0
    max: 22
    initial: 0
  - name: maxCurrentAmps
    type: number
    min: 6
    max: 32
    initial: 32
  - name: connectorLocked
    type: boolean
    initial: false
commands:
  - name: START_TRANSACTION
    effects:
      status: CHARGING
      connectorLocked: true
      chargingPower: 11
    userControllable: true
  - name: STOP_TRANSACTION
    effects:
      status: AVAILABLE
      connectorLocked: false
      chargingPower: 0
    userControllable: true
  - name: EMERGENCY_STOP
    effects:
      status: FAULTED
      connectorLocked: false
      chargingPower: 0
    userControllable: true
  - name: SET_CURRENT_LIMIT
    params:
      limit:
        type: number
        min: 6
        max: 32
    effects:
      maxCurrentAmps: "$limit"
    userControllable: true
```

---

## Solar Inverter

**File:** `examples/solar-inverter.yaml`

Grid-connected solar generation unit with DC/AC conversion, grid synchronization, and remote curtailment controls.

```yaml
type: solar-inverter
version: 1
description: Grid-tied smart photovoltaic solar inverter
telemetry:
  - name: solarIrradiance
    unit: "W/m2"
    min: 0
    max: 1200
    noise: 10
    intervalSec: 5
  - name: dcPowerKw
    unit: "kW"
    min: 0
    max: 10
    noise: 0.1
    intervalSec: 5
    follows:
      state: targetPowerKw
      when:
        mode: MPPT
      rate: 0.3
  - name: acPowerKw
    unit: "kW"
    min: 0
    max: 10
    noise: 0.1
    intervalSec: 5
  - name: gridFrequencyHz
    unit: "Hz"
    min: 49.0
    max: 51.0
    noise: 0.04
    intervalSec: 5
state:
  - name: mode
    type: enum
    values: [STANDBY, MPPT, CURTAILED, FAULT]
    initial: MPPT
  - name: gridConnected
    type: boolean
    initial: true
  - name: targetPowerKw
    type: number
    min: 0
    max: 10
    initial: 5
commands:
  - name: DISCONNECT_GRID
    effects:
      gridConnected: false
      mode: STANDBY
      targetPowerKw: 0
    userControllable: true
  - name: CONNECT_GRID
    effects:
      gridConnected: true
      mode: MPPT
      targetPowerKw: 5
    userControllable: true
  - name: SET_CURTAILMENT
    params:
      pct:
        type: number
        min: 0
        max: 100
    effects:
      mode: CURTAILED
      curtailmentPct: "$pct"
    userControllable: true
```

---

## Machinery Vibration Sensor

**File:** `examples/vibration-sensor.yaml`

Tri-axial accelerometer and acoustic sensor for predictive maintenance and condition monitoring in factories.

```yaml
type: vibration-sensor
version: 1
description: Industrial tri-axial machinery vibration and acoustic sensor
telemetry:
  - name: vibrationRms
    unit: "mm/s"
    min: 0
    max: 50
    noise: 0.2
    intervalSec: 2
    follows:
      state: baselineVibration
      when:
        machineStatus: RUNNING
      rate: 0.4
  - name: peakAcceleration
    unit: "g"
    min: 0
    max: 20
    noise: 0.15
    intervalSec: 2
  - name: acousticEmission
    unit: "dB"
    min: 20
    max: 110
    noise: 1.2
    intervalSec: 2
state:
  - name: machineStatus
    type: enum
    values: [OFF, IDLE, RUNNING, HIGH_LOAD, FAULT]
    initial: RUNNING
  - name: baselineVibration
    type: number
    min: 0
    max: 50
    initial: 2.5
commands:
  - name: SET_MACHINE_STATUS
    params:
      status:
        type: enum
        values: [OFF, IDLE, RUNNING, HIGH_LOAD, FAULT]
    effects:
      machineStatus: "$status"
    userControllable: true
  - name: SIMULATE_ANOMALY
    effects:
      machineStatus: HIGH_LOAD
      baselineVibration: 25
      alarmTriggered: true
    userControllable: true
```

---

## Cold-Chain Asset Tracker

**File:** `examples/cold-chain-tracker.yaml`

Pharma and perishable food transport tracker monitoring storage temperatures, refrigeration modes, door activity, and battery status.

```yaml
type: cold-chain-tracker
version: 1
description: Refrigerated transport and pharmaceutical cold-chain asset tracker
telemetry:
  - name: cargoTempC
    unit: "C"
    min: -30
    max: 25
    noise: 0.2
    intervalSec: 5
    follows:
      state: targetTempC
      when:
        coolerState: COOLING
      rate: 0.1
  - name: ambientTempC
    unit: "C"
    min: -10
    max: 45
    noise: 0.3
    intervalSec: 10
  - name: batteryPct
    unit: "%"
    min: 0
    max: 100
    noise: 0.05
    intervalSec: 30
state:
  - name: coolerState
    type: enum
    values: [COOLING, DEFROST, IDLE, OFF]
    initial: COOLING
  - name: doorOpen
    type: boolean
    initial: false
  - name: targetTempC
    type: number
    min: -30
    max: 15
    initial: -18
commands:
  - name: OPEN_DOOR
    effects:
      doorOpen: true
    userControllable: true
  - name: CLOSE_DOOR
    effects:
      doorOpen: false
    userControllable: true
  - name: SET_TARGET_TEMP
    params:
      temp:
        type: number
        min: -30
        max: 15
    effects:
      targetTempC: "$temp"
    userControllable: true
```

---

## Smart Water Meter

**File:** `examples/water-meter.yaml`

Utility water meter with automated valve shutoff, flow rate telemetry, and cumulative volume tracking.

---

## Smart Smoke Alarm

**File:** `examples/smoke-alarm.yaml`

Life-safety sensor with particulate density monitoring, self-testing, and siren actuator.

---

## Creating Your Own Spec

Use these examples as templates. The general process:

1. Decide what your device **measures** (telemetry) and what it **does** (state + commands)
2. Define the telemetry metrics with appropriate ranges and intervals
3. Define the state fields with types and initial values
4. Define commands with parameters, effects, and access control
5. Validate by starting the runtime: it will report all errors at startup
