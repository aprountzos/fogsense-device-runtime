# Cross-transport delivery and security roadmap

This document defines the delivery and security guarantees expected from FogSense regardless of whether a device communicates with its gateway over HTTP or MQTT 5.

It is a target contract and implementation plan. It does not claim that every capability below is implemented today.

## 1. Transport-neutral guarantees

The runtime should provide the same application-level guarantees across HTTP and MQTT:

- **At-least-once outbound delivery:** a message remains in the device outbox until the gateway application acknowledges acceptance.
- **Crash recovery:** queued messages survive a device process restart when durable outbox storage is enabled.
- **Stable message identity:** retries and process restarts must not turn one logical message into a new message. Gateways deduplicate using a stable message identifier.
- **Ordering:** messages from one device are delivered in sequence order where the transport and gateway permit it. The contract must define behavior when a message is permanently rejected.
- **Bounded storage:** the outbox has configurable size limits and an explicit overflow policy. Silent loss is not acceptable; dropped-message counters and logs are required.
- **Explicit rejection:** permanent validation/authentication failures are distinguished from transient delivery failures. Transient failures are retried with bounded exponential backoff.
- **Observability:** queue depth, oldest-message age, retry count, last successful delivery, and dropped-message count are inspectable.

At-least-once delivery can produce duplicates. Exactly-once delivery is not promised. Consumers must make processing idempotent.

## 2. Transport-specific acknowledgement

### HTTP

- A successful 2xx response means the gateway application durably accepted the message, not merely that an HTTP handler received the request.
- Transient network errors, 408, 429, and 5xx responses are retried.
- Permanent 4xx responses are surfaced as rejected messages and must not be silently treated as successful delivery.
- HTTPS is required outside isolated local development.

### MQTT 5

- QoS 1 PUBACK means the broker accepted the publication; it is not the gateway application's acknowledgement.
- The gateway application must acknowledge message acceptance after its own durable handoff.
- Registration acknowledgement remains a separate application-level handshake.
- Device subscriptions and publications must be scoped to the device's own topics by broker ACLs.
- MQTT sessions, QoS, and broker persistence improve broker-side recovery but do not replace the device outbox or application acknowledgement.

## 3. Stable message identity and deduplication

The current envelope identifies a message within one runtime boot using deviceId, bootId, and seq. A durable outbox must preserve identity across process restarts. The implementation must therefore introduce a stable messageId generated once when a message is enqueued and retained unchanged for every retry.

Gateways should deduplicate by the pair (deviceId, messageId). The existing boot and sequence fields remain useful for diagnostics and ordering, but must not be the only deduplication key for durable replay.

Command execution remains idempotent by commandId. A gateway must retain its deduplication records for at least the maximum outbox replay window.

## 4. Security baseline

Security is transport-neutral at the policy level, but the mechanisms differ:

| Concern | HTTP | MQTT |
| --- | --- | --- |
| Device-to-gateway authentication | Unique device token or stronger workload identity | Unique broker username/password or client certificate |
| Encryption in transit | HTTPS/TLS | MQTT over TLS (mqtts://) |
| Authorization | Gateway validates device identity and allowed operations | Broker ACL restricts each device to its own topics |
| Secret handling | Secret manager / mounted secret | Secret manager / mounted secret |
| Replay/duplicate protection | Stable message ID and idempotent processing | Stable message ID, QoS 1 duplicate tolerance, idempotent processing |

Production deployments must not rely on anonymous access or shared demo credentials. The local MQTT playground may remain intentionally anonymous, but must be clearly labelled development-only.

The local device HTTP API is a separate security boundary from the device-to-gateway transport. Its token, instructor control token, HTTP gateway credential, and MQTT broker credential must remain distinct.

## 5. Implementation phases and progress

1. **Durable outbox shared by HTTP and MQTT — implemented, with operational hardening remaining**
   - A disk-backed JSON outbox is shared by both transports.
   - Enqueue, stable message IDs, sequence state, and acknowledgements are atomically persisted.
   - The default path is `data/outbox-<DEVICE_ID>.json`; override it with `OUTBOX_PATH`.
   - The outbox is bounded to 500 messages. When full, the newest message is rejected and counted; older queued messages are not evicted.
   - Corrupt or unsupported outbox state prevents runtime startup rather than silently starting with an empty queue.
   - The container examples mount persistent volumes for device outboxes.
   - Remaining hardening: cross-process file locking, configurable capacity/retention, disk-full fault tests, and stronger filesystem portability validation.

2. **Gateway acceptance and deduplication — implemented in the reference gateways; production transaction boundary remains**
   - HTTP 2xx is treated as application acceptance. The reference HTTP gateway commits the accepted payload and `(deviceId, messageId)` key together in SQLite before returning success.
   - MQTT QoS 1 PUBACK is not treated as application acceptance. The device waits for `fogsense/v1/devices/{deviceId}/delivery/ack` with a matching `messageId` and `accepted: true`.
   - The reference MQTT gateway commits the accepted payload and deduplication key together in the same SQLite store before publishing the application acknowledgement.
   - Both reference gateways use the shared `pkg/gatewaystore` implementation and deduplicate by `(deviceId, messageId)`.
   - The durable accepted-message journal is transactional. The mock gateways' in-memory device aggregates/debug views are still demonstration state and are not rebuilt from the journal after restart; production consumers should project the journal into their durable business store.

3. **Production security profiles — implemented for MQTT examples; HTTP deployment guidance remains**
   - MQTT supports system trust or a custom `MQTT_CA_FILE`, per-device username/password, and optional client certificate/key for mutual TLS.
   - `mqtts://` requires username/password or a client certificate.
   - Added a TLS Mosquitto configuration, per-device ACL example, and a secure Compose profile.
   - The anonymous MQTT playground remains explicitly local-development-only.
   - HTTP uses HTTPS/TLS and its separate gateway token; production deployments must configure HTTPS and unique device credentials.
   - Remaining: automated broker ACL integration tests, certificate rotation, and an equivalent fully automated HTTPS test profile.

4. **Operational visibility and lifecycle — partially implemented**
   - The control/status response exposes queue depth, dropped count, sequence, outbox path/health, oldest-message age, retry failures, last successful delivery and last error.
   - Both mock gateways expose readiness health endpoints that include deduplication-store availability; the MQTT Compose healthcheck checks subscription readiness.
   - Transient HTTP failures (including 408, 429 and 5xx) and MQTT acknowledgement timeouts retain messages for retry. Permanent rejections are counted and removed.
   - Remaining: disk-space thresholds/alerts, durable payload journaling in the production gateway, and shutdown/disk-full fault-injection coverage.

5. **Conformance and local test suite — in progress**
   - Added unit tests for outbox recovery/corruption, stable IDs, queue capacity, HTTP acknowledgement/retry behavior, MQTT acknowledgement matching, custom CA configuration, and gateway deduplication persistence.
   - CI now exercises MQTT broker restart recovery and device-process restart with a pending durable outbox.
   - Remaining: broader HTTP/MQTT conformance parity, secure-broker/ACL integration tests, disk-full fault injection, and the final PowerShell test runner.

## 6. Current implementation boundaries

- Delivery is **at least once**, not exactly once. Consumers must be idempotent.
- The device outbox is durable only when its file or mounted volume survives container replacement. Deleting a named volume deletes pending messages.
- Device SQLite checkpoints include metric progression, last readings, and telemetry PRNG state. Generated telemetry batches remain in a bounded device-side pending queue until their stable message IDs are durably handed to the transport outbox; if handoff cannot complete, telemetry generation applies backpressure.
- MQTT inbound QoS 1 command deliveries use manual broker acknowledgement. The device acknowledges a command only after command execution/idempotency state is committed and the result is durably queued in the outbound outbox. A resumable 24-hour MQTT session allows the broker to redeliver unacknowledged commands after process restart, provided the broker retains its session state.
- One runtime process must own a given device outbox path; do not mount the same file into multiple replicas.
- The reference gateways persist accepted telemetry, events, and command acknowledgements together with their deduplication keys in SQLite. Their in-memory device aggregates/debug views are not yet reconstructed from that journal after restart.
- The local anonymous Mosquitto profile is for development only. Use the secure profile or an appropriately managed broker in production. The HTTP mock gateway accepts one shared demo token; production gateways must provision and verify unique per-device credentials.
- The local HTTP device API remains a separate security boundary from the gateway transport.
