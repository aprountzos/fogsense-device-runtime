---
id: telemetry
title: Telemetry Metrics
sidebar_label: Telemetry
sidebar_position: 2
---

# Telemetry Metrics

Telemetry metrics define the data a device generates and sends to the gateway at regular intervals. Each metric simulates a sensor reading with configurable behavior.

## Metric Definition

```yaml
name: temperature
unit: "°C"
min: 18
max: 28
noise: 0.4
anomalyRate: 0.03
intervalSec: 5
follows:
  state: targetTemp
  when:
    power: "ON"
  rate: 0.2
```

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | `string` | ✅ | - | Unique metric name (`[A-Za-z][A-Za-z0-9_]{0,31}`) |
| `unit` | `string` | ❌ | `""` | Unit label (e.g. `"°C"`, `"%"`, `"lux"`) |
| `min` | `number` | ✅ | - | Minimum normal value |
| `max` | `number` | ✅ | - | Maximum normal value (`min < max` required) |
| `noise` | `number` | ❌ | `0` | Standard deviation of Gaussian noise added to each reading |
| `anomalyRate` | `number` | ❌ | `0` | Probability (0 to 1) that a reading is an anomaly (out-of-range) |
| `intervalSec` | `number` | ❌ | `5` | Seconds between readings (1 to 3600) |
| `follows` | `object` | ❌ | - | State-following configuration (see below) |

## How Values Are Generated

Each metric maintains an internal value `x` that evolves over time:

### Normal Operation

1. **Random walk**: The value drifts randomly within `[min, max]`
2. **Gaussian noise**: A noise term with standard deviation `noise` is added
3. **Rounding**: The final value is rounded to 2 decimal places

### Anomalies

With probability `anomalyRate`, the reading is an **anomaly**:
- The value is pushed **outside** the `[min, max]` range
- It goes above `max` or below `min` by 10% to 50% of the range
- This simulates real-world sensor malfunctions or extreme events

### State Following (`follows`)

When configured, a metric can **track a state field value**:

```yaml
follows:
  state: targetTemp      # numeric state field to track
  when:
    power: "ON"          # conditions that must hold
  rate: 0.2              # convergence rate (0 < rate ≤ 1)
```

**How it works:**
- When **all** `when` conditions are satisfied, the metric moves toward the value of the specified state field
- The `rate` controls convergence speed (higher = faster)
- When conditions are **not** met, the metric drifts back toward the center of `[min, max]`

**Example:** A thermostat's temperature metric follows `targetTemp` when `power` is `ON`. When the user sets `targetTemp` to 22°C, the temperature gradually converges toward 22°C.

:::info
**Rules**

- `follows.state` must reference an existing **numeric** state field
- Every key in `follows.when` must be an existing state field with a valid value
- `follows.rate` defaults to `0.15` if omitted
:::

## Telemetry Delivery

Each telemetry reading is sent to the gateway as part of a batch:

```json
{
  "deviceId": "sensor-1",
  "bootId": "abc-123",
  "seq": 42,
  "ts": "2025-01-15T10:30:00.000Z",
  "readings": [
    { "metric": "temperature", "value": 22.35, "unit": "°C", "ts": "2025-01-15T10:30:00.000Z" },
    { "metric": "humidity", "value": 45.12, "unit": "%", "ts": "2025-01-15T10:30:00.000Z" }
  ]
}
```

- Multiple metrics due at the same time are batched together
- Each batch has a unique, strictly increasing `seq` number
- Delivery is guaranteed **at least once**, in order
- While the device is **OFFLINE**, no telemetry is generated
