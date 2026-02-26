# Security And Performance Hardening Guide

## Objective

This document defines production-grade hardening for GoWA API with concrete controls, tuning knobs, and validation strategy.

## Threat Model (Practical)

- Unauthorized API usage: leaked credentials/tokens.
- Abuse/DoS on REST endpoints.
- Webhook replay/flood and remote endpoint instability.
- Data leakage through logs/config dumps.
- Browser-origin abuse in cross-origin deployments.

## Controls Implemented

### 1) Authentication layers

- Basic auth (`APP_BASIC_AUTH`) and token auth (`APP_AUTH_TOKEN`) are supported.
- Chatwoot inbound webhook can be isolated with `CHATWOOT_WEBHOOK_TOKEN`.

### 2) Security headers

- Enabled by default (`APP_SECURITY_HEADERS=true`).
- Adds:
  - `X-Frame-Options: SAMEORIGIN`
  - `X-Content-Type-Options: nosniff`
  - `X-XSS-Protection`
  - `Referrer-Policy`
  - HSTS (`Strict-Transport-Security`)

### 3) Request rate limiting

- Optional global limiter by client IP:
  - `APP_RATE_LIMIT_ENABLED=true`
  - `APP_RATE_LIMIT_MAX=120`
  - `APP_RATE_LIMIT_WINDOW_SEC=60`
- Excludes paths with different traffic pattern:
  - `/chatwoot/webhook`
  - `/statics`, `/assets`, `/components`
  - `/ws`

### 4) CORS tightening

- CORS is explicit-only (`APP_CORS_ORIGINS`).
- If unset, CORS middleware is not registered.
- Avoid `*` in authenticated production environments.

### 5) Webhook delivery performance and resilience

- Outbound webhook HTTP client now uses pooled persistent connections.
- Retry backoff respects context cancellation.
- Response body is closed immediately on each attempt (prevents descriptor/memory accumulation).

### 6) Secret hygiene

- Startup no longer logs entire Viper settings.
- `WHATSAPP_WEBHOOK_SECRET` is required when webhook forwarding is configured.

## Recommended Production Configuration

```env
APP_DEBUG=false
APP_SECURITY_HEADERS=true
APP_AUTH_TOKEN=<long-random-token>
APP_CORS_ORIGINS=https://your-frontend.example.com
APP_RATE_LIMIT_ENABLED=true
APP_RATE_LIMIT_MAX=180
APP_RATE_LIMIT_WINDOW_SEC=60
WHATSAPP_AUTO_DOWNLOAD_STATUS_MEDIA=false
WHATSAPP_HISTORY_SYNC_DUMP_ENABLED=false
WHATSAPP_WEBHOOK=https://your-receiver.example.com/events
WHATSAPP_WEBHOOK_SECRET=<long-random-secret>
CHATWOOT_SYNC_INCLUDE_STATUS=false
CHATWOOT_SYNC_MAX_MEDIA_FILE_SIZE=10000000
```

## Performance Tuning Playbook

- High webhook throughput:
  - keep `WHATSAPP_WEBHOOK` receivers close (low RTT)
  - avoid enabling insecure TLS except dev
- API peak traffic:
  - increase `APP_RATE_LIMIT_MAX` with load tests
  - keep payload limits aligned with media size policy
- Database:
  - keep WAL enabled for SQLite workloads
  - use Postgres for high concurrency + large chat history

## Validation Checklist

- Run:
  - `go test ./...`
  - `go vet ./...`
- Confirm:
  - auth required endpoints reject unauthenticated requests
  - rate limiter emits `429 RATE_LIMITED` under burst
  - webhook delivery succeeds with retry behavior under transient failures
  - `GET /healthz` responds `200` for liveness/readiness probes

## Operational Metrics To Add Next

- Request latency percentiles by route (`p50/p95/p99`)
- Outbound webhook success/fail counts and retry distribution
- Authentication failures by source IP
- Rate-limit hits by endpoint
