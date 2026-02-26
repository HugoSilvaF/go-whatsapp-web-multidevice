# Feature Roadmap (Technical)

This roadmap prioritizes features with high operational impact and low regression risk.

## P0 - High Impact

### 1) API Keys with scopes and rotation

Status:
- Implemented in current codebase (`/auth/keys*`, scoped auth middleware, key rotation/revocation).

Goal:
- Replace single shared token with multi-key auth and granular permissions.

Spec:
- Storage table: `api_keys(id, name, hash, scopes, expires_at, created_at, revoked_at)`
- Scopes examples:
  - `messages:send`
  - `messages:manage`
  - `groups:manage`
  - `devices:manage`
  - `chatwoot:sync`
- Endpoints:
  - `POST /auth/keys`
  - `GET /auth/keys`
  - `DELETE /auth/keys/:id`
  - `POST /auth/keys/:id/rotate`

### 2) Idempotency for send endpoints

Goal:
- Prevent duplicate messages on client retries.

Spec:
- Header: `Idempotency-Key`
- Persist request fingerprint + response for TTL window.
- Apply to `/send/*` endpoints.

### 3) Webhook Delivery Queue + Dead Letter Queue

Goal:
- Prevent event loss when external webhook receiver is unstable.

Spec:
- Queue storage with retry metadata.
- Worker with exponential backoff + max retry.
- DLQ endpoint: `GET /webhook/dlq`.
- Replay endpoint: `POST /webhook/dlq/:id/retry`.

## P1 - Performance and Operability

### 4) Batch send endpoint

Goal:
- Reduce roundtrips for campaigns and queue-based sending.

Spec:
- `POST /send/batch`
- Body: array of send payloads
- Concurrency controls per device and per destination.
- Partial-success response with per-item status.

### 5) Streaming exports for chat history

Goal:
- Avoid memory spikes when exporting large conversations.

Spec:
- `GET /chat/:chat_jid/export?format=jsonl|csv`
- Stream chunks instead of building full payload in memory.

### 6) OpenTelemetry traces + Prometheus metrics

Goal:
- Improve incident response and capacity planning.

Spec:
- Trace HTTP handlers, WhatsApp send operations, webhook forward calls.
- Metrics:
  - request latency per route
  - webhook retry count
  - queue depth
  - auth failures

## P2 - Product Features

### 7) Scheduled messages

Goal:
- Send messages at future timestamp with timezone support.

Spec:
- `POST /send/schedule`
- `GET /send/schedule`
- `DELETE /send/schedule/:id`

### 8) Message templates with variables

Goal:
- Reusable content for operations/CRM workflows.

Spec:
- `POST /templates`
- `GET /templates`
- `POST /send/template`

### 9) Delivery analytics dashboard endpoint

Goal:
- Expose aggregate stats without direct DB querying.

Spec:
- `GET /analytics/messages?from=&to=&device_id=`
- Returns sent/failed/retried/read metrics.

## Delivery Strategy

- Implement P0 features behind feature flags.
- Add integration tests for auth/idempotency/queue semantics.
- Ship docs and migration guide with each feature.
