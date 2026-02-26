# Implementation Change Log And Migration Guide

This document consolidates what was implemented, why it was implemented, compatibility impact, and how to migrate safely.

Scope of this document:
- Auth hardening with scoped API keys and rotation.
- Chatwoot sync hardening for performance and overload prevention.
- Security/performance controls and operational validation updates.

## 1) What Was Implemented And Why

### 1.1 Scoped API Keys + Rotation (P0 roadmap item)

Implemented:
- New API key service with persistent metadata and hashed key storage.
- New management routes:
  - `GET /auth/keys`
  - `POST /auth/keys`
  - `DELETE /auth/keys/:id`
  - `POST /auth/keys/:id/rotate`
- Scope-based authorization middleware for API-key-authenticated requests.

Why:
- Replace broad shared-token usage with least-privilege access.
- Enable controlled key lifecycle (create, rotate, revoke) without downtime.
- Reduce blast radius when a key leaks.

Technical behavior:
- Key format: `gowa.<id>.<secret>`.
- Only a hash is persisted (`key_hash`), never the plaintext secret.
- Secret is returned only on create/rotate response.
- Expired/revoked keys are rejected at auth middleware.
- Basic auth and shared token keep full access (backward compatibility path).

### 1.2 Chatwoot Sync Performance Guardrails

Implemented:
- Sync options to constrain heavy imports:
  - `include_status` / `CHATWOOT_SYNC_INCLUDE_STATUS` (default `false`)
  - `max_messages_per_chat`
  - `batch_size`
  - `delay_ms`
  - `max_media_file_size`
- Status/story chat (`status@broadcast`) excluded by default in sync flow.
- Media download skip by configured max file size threshold.
- Sync-in-progress protection per device (`409` on concurrent sync trigger).

Why:
- Prevent overload and lockups reported during history sync.
- Avoid expensive story/status media downloads by default.
- Bound CPU/network/disk pressure during large backfills.

### 1.3 Security Controls And Configurability

Implemented:
- Security headers middleware (enabled by default).
- Optional IP rate limiting middleware.
- CORS allow-list configuration.
- Optional Chatwoot inbound webhook token validation (`X-Chatwoot-Token` or `token` query).
- Safer defaults for data volume/sensitivity:
  - `WHATSAPP_AUTO_DOWNLOAD_STATUS_MEDIA=false`
  - `WHATSAPP_HISTORY_SYNC_DUMP_ENABLED=false`

Why:
- Improve default posture against abuse, accidental data exposure, and noisy traffic.
- Make production hardening explicit and configurable.

### 1.4 Quality Validation Improvements

Implemented:
- CI improvements:
  - deterministic tests with `-count=1`
  - race-test step in CI for critical packages (`infrastructure/apikey`, `ui/rest/middleware`)
- Documentation updates for local test/race prerequisites.

Why:
- Catch concurrency regressions earlier.
- Reduce false confidence from cached or environment-specific test behavior.

## 2) Compatibility And Breaking Changes

Classification legend:
- `Breaking`: existing integrations can fail without changes.
- `Behavioral`: defaults/behavior changed but API contract still valid.
- `Additive`: new optional feature, no break by default.

### 2.1 Compatibility Matrix

| Change | Type | Impact | Required action |
|---|---|---|---|
| Scoped API keys and scope checks | Additive | No break if you keep Basic/shared token. API keys now can be restricted by scopes. | For new API keys, assign correct scopes per integration. |
| `/auth/keys*` endpoints added | Additive | No break. New management surface. | Protect `auth:manage` keys; do not expose in public clients. |
| Chatwoot sync excludes status/story by default (`include_status=false`) | Behavioral | Existing flows that expected story sync will now skip it unless explicitly enabled. | Set `CHATWOOT_SYNC_INCLUDE_STATUS=true` only if needed and capacity-tested. |
| Sync media size cap (`CHATWOOT_SYNC_MAX_MEDIA_FILE_SIZE`) | Behavioral | Large media may be skipped with annotation in message content. | Tune cap based on bandwidth/storage policy. |
| Security headers enabled by default | Behavioral (potentially breaking for some UIs) | Embedded iframe/admin panel scenarios can be affected depending on header policy. | Validate browser embedding behavior; adjust deployment strategy if required. |
| Optional webhook token validation for Chatwoot | Additive | No break unless token is configured and sender does not provide it. | If set, configure Chatwoot webhook sender with matching token. |
| Rate limiting middleware | Additive | No break unless enabled; then bursts can return `429`. | Configure limits for your traffic profile and retry policy. |

## 3) Step-By-Step Migration

### Step 0 - Prepare
1. Backup `.env` and deployment manifests.
2. Validate current auth mode in use (Basic, shared token, or API key).
3. Confirm production traffic baseline (req/s and peak burst).

### Step 1 - Roll Out Safe Defaults
1. Ensure these defaults are explicit in config:
   - `WHATSAPP_AUTO_DOWNLOAD_STATUS_MEDIA=false`
   - `WHATSAPP_HISTORY_SYNC_DUMP_ENABLED=false`
   - `CHATWOOT_SYNC_INCLUDE_STATUS=false`
2. Deploy and verify normal traffic health (`GET /healthz`).

### Step 2 - Migrate To Scoped API Keys
1. Create scoped key with minimum scopes required:
   - `POST /auth/keys`
2. Update consumer integration with new `X-API-Key`.
3. Validate required endpoints return `200` and out-of-scope routes return `403 FORBIDDEN_SCOPE`.
4. Revoke old key:
   - `DELETE /auth/keys/:id`

### Step 3 - Adopt Rotation Policy
1. Rotate key:
   - `POST /auth/keys/:id/rotate`
2. Update consumers with new secret.
3. Verify old secret no longer authenticates.
4. Record rotation date and set next rotation window.

### Step 4 - Tune Chatwoot Sync For Capacity
1. Start with conservative values:
   - `CHATWOOT_SYNC_BATCH_SIZE=10`
   - `CHATWOOT_SYNC_DELAY_MS=500`
   - `CHATWOOT_SYNC_MAX_MEDIA_FILE_SIZE=10000000` (10 MB)
2. Trigger controlled sync:
   - `POST /chatwoot/sync`
3. Monitor sync status:
   - `GET /chatwoot/sync/status`
4. Increase throughput gradually only after stable run.

### Step 5 - Enable Runtime Protection
1. Enable rate limit:
   - `APP_RATE_LIMIT_ENABLED=true`
2. Restrict CORS origins:
   - `APP_CORS_ORIGINS=https://your-frontend.example`
3. If using Chatwoot webhook protection:
   - set `CHATWOOT_WEBHOOK_TOKEN` and validate inbound requests include it.

## 4) Operational Validation Checklist

Auth:
- Invalid credentials return `401 UNAUTHORIZED`.
- API key without required scope returns `403 FORBIDDEN_SCOPE`.
- Revoked key fails authentication.
- Rotated old secret fails authentication.

Chatwoot sync:
- Concurrent sync request returns `409`.
- Status chats are skipped when `include_status=false`.
- Large media is skipped when size exceeds cap.

Security/performance:
- Rate limiting returns `429` under burst (when enabled).
- Security headers are present when enabled.
- CORS only allows configured origins.

Build/test:
- `cd src && go vet ./...`
- `cd src && go test -count=1 ./...`
- CI race tests are enabled for critical packages.

## 5) References

- Route contracts: [routes.md](./routes.md)
- API key internals and runbooks: [api-keys.md](./api-keys.md)
- Chatwoot integration and sync tuning: [chatwoot.md](./chatwoot.md)
- Security/performance controls: [security-performance-hardening.md](./security-performance-hardening.md)
- OpenAPI contract: [openapi.yaml](./openapi.yaml)
