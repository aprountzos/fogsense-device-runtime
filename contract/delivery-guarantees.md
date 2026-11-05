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

## 5. Implementation phases

1. **Durable outbox shared by HTTP and MQTT**
   - Add a disk-backed outbox with atomic updates and recovery.
   - Add stable message IDs and persist sequence/outbox metadata.
   - Define bounded capacity, overflow behavior, retry classification, and permanent rejection handling.
   - Keep the existing in-memory mode available for ephemeral demos if explicitly selected.
2. **Gateway acceptance and deduplication contract**
   - Add application-level acknowledgements for HTTP and MQTT data messages.
   - Deduplicate by (deviceId, messageId) before side effects.
   - Persist gateway acceptance/deduplication state in the reference gateway used for integration tests.
3. **Production security profiles**
   - Add a secure Mosquitto example with TLS, per-device credentials, and topic ACLs.
   - Make insecure local-demo settings explicit.
   - Add tests for missing credentials, mismatched credentials, unauthorized topic access, and TLS configuration.
   - Document HTTPS deployment for the HTTP path and keep local API/control credentials separate.
4. **Operational visibility and lifecycle**
   - Expose outbox and transport health through the device health/control diagnostics.
   - Provide a meaningful health signal for the MQTT mock gateway rather than applying the device HTTP health check to it.
   - Cover shutdown, reconnect, broker outage, process restart, disk-full, and queue-capacity behavior.
5. **Conformance, documentation, and local test suite**
   - Run the same logical delivery scenarios against HTTP and MQTT.
   - Document expected results and failure semantics.
   - Provide a PowerShell-first local test runner and manual fault-injection commands.

## 6. Current implementation status at the time this contract was added

The current runtime has a shared bounded in-memory outbound queue for HTTP and MQTT. It retries transient failures while the process remains alive. MQTT uses QoS 1, registration application acknowledgements, reconnect/re-subscription, and an application command-ack queue.

The following are not yet complete: durable outbox recovery across process restarts, stable message IDs across restarts, gateway application acknowledgements for every telemetry/event/command-ack message, durable gateway-side deduplication, and an enforced production MQTT TLS/credential/ACL profile. The local Mosquitto playground deliberately permits anonymous access and disables broker persistence.

This status section must be updated as implementation phases are completed.
