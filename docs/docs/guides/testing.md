---
id: testing
title: Testing
sidebar_label: Testing
sidebar_position: 3
---

# Testing

The FogSense Device Runtime includes a comprehensive test suite that verifies spec validation, device behavior, and contract conformance.

## Unit Tests

Run the native Go test suite covering PRNG Mulberry32, specification validation, and device engine logic:

```bash
go test -v ./...
```

### Test Coverage

The Go test suite covers:

#### Spec Validation (`pkg/spec/spec_test.go`)

| Test | What It Verifies |
|---|---|
| Example specs are valid | All files in `examples/` pass validation |
| Defaults are applied | Missing `intervalSec`, `noise`, `offlineRate` get defaults |
| State initial defaults | Enum → first value, number → `min`, boolean → `false` |
| Non-object rejection | Arrays, strings, etc. are rejected |
| Unknown fields | Extra top-level fields are caught |
| Name validation | Bad type names, field names are rejected |
| Version validation | Non-integer or `< 1` versions are rejected |
| Empty device | At least one of telemetry/state required |
| Duplicate names | Duplicate metric/state/command names are caught |
| Range validation | `min >= max` is rejected |
| Anomaly rate | Values outside `[0, 1]` are rejected |
| Interval bounds | Values outside `[1, 3600]` are rejected |
| Array limits | Too many metrics, params, etc. |
| Enum rules | Missing values, `$`-prefix, empty strings |
| Initial value | Must fit declared type/range |
| Effects validation | Unknown fields, type mismatches, range violations |
| Param references | `$param` must exist and match types |
| Follows rules | Must reference numeric state, valid conditions |

#### Device Behavior (`pkg/device/device_test.go`)

| Test | What It Verifies |
|---|---|
| Telemetry emission | Correct number of readings per metric per interval |
| Anomaly generation | `anomalyRate=1` always produces out-of-range values |
| Reproducibility | Same seed → same values |
| Command execution | Effects applied, events emitted, state returned |
| No-change detection | No event when state doesn't change |
| Idempotency | Same `commandId` → cached result with `duplicate: true` |
| Invalid commands | Correct rejection reasons |
| Offline commands | `503 DEVICE_OFFLINE`, not cached |
| Fault injection | Offline/spike/flood with correct timing |
| State following | Metric converges toward state target |
| Time scale | `timeScale` accelerates device time |
| Info view | Self-describing output with `userControllable` |
| Control validation | Bad fault inputs are rejected |

## Contract Conformance Checks

The mock gateway's `--check` mode validates that a running device conforms to the full API contract. See the [Mock Gateway](./mock-gateway.md#conformance-mode---check) guide for details.

```bash
# Terminal 1
go run ./cmd/mock-gateway --port 8081 --device-token t --check &

# Terminal 2
DEVICE_ID=d1 \
GATEWAY_URL=http://127.0.0.1:8081 \
DEVICE_TOKEN=t \
ADVERTISE_URL=http://127.0.0.1:8080 \
DEVICE_SPEC_FILE=examples/thermostat.yaml \
go run .
```

### Testing All Example Specs

Run conformance checks against each example spec:

```bash
for spec in examples/*.yaml; do
  echo "=== Testing $spec ==="
  go run ./cmd/mock-gateway --port 8081 --device-token t --check &
  GW_PID=$!
  sleep 1
  DEVICE_ID=test-device \
  GATEWAY_URL=http://127.0.0.1:8081 \
  DEVICE_TOKEN=t \
  ADVERTISE_URL=http://127.0.0.1:8080 \
  DEVICE_SPEC_FILE=$spec \
  go run . &
  DEV_PID=$!
  wait $GW_PID
  kill $DEV_PID 2>/dev/null
  echo ""
done
```

## Testing with `TIME_SCALE`

Speed up testing by increasing the time scale:

```bash
TIME_SCALE=10 SEED=42 DEVICE_ID=fast-test \
GATEWAY_URL=http://127.0.0.1:8081 \
DEVICE_SPEC_FILE=examples/thermostat.yaml \
go run .
```

This makes 60 seconds of device time pass in just 6 seconds of real time, useful for testing offline recovery, telemetry convergence, and other time-dependent behaviors.

## Reproducible Testing

Use the `SEED` environment variable for deterministic output:

```bash
SEED=42 go run .
```

Two runs with the same seed and time scale produce identical telemetry values. This is useful for:
- Regression testing
- Debugging specific sequences
- Comparing behavior across runtime implementations
