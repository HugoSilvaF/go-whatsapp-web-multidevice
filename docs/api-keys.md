# API Keys (Scoped) - Technical Guide

## Overview

Scoped API keys allow least-privilege access to the REST API.

- Key format: `gowa.<id>.<secret>`
- Secret is shown only on create/rotate.
- Stored securely as SHA-256 hash (plain secret is not persisted).
- Supports expiration and revocation.
- Supports key rotation without changing key ID.

## Authentication Modes And Precedence

The API currently accepts these modes:

1. Basic auth (`Authorization: Basic ...`)
2. Shared bearer token (`APP_AUTH_TOKEN`)
3. Scoped API key (`X-API-Key` or `Authorization: Bearer gowa...`)

Behavior:

- Basic/shared token = full access.
- Scoped API key = route access controlled by scopes.

## Data Model

`api_keys`
- `id` (PK)
- `name`
- `key_hash`
- `scopes` (comma-separated)
- `expires_at` (nullable)
- `revoked_at` (nullable)
- `last_used_at` (nullable)
- `created_at`
- `updated_at`

Security properties:

- Only hash is stored (`key_hash`), never raw key secret.
- Revoked key is denied immediately.
- Expired key is denied immediately.
- Last usage is tracked (`last_used_at`) for operational audits.

## Management Endpoints

- `POST /auth/keys`
  - body: `{ "name": "...", "scopes": ["messages:send"], "expires_in_days": 30 }`
  - returns metadata + `api_key`
- `GET /auth/keys?include_revoked=false`
  - returns metadata only
- `DELETE /auth/keys/:id`
  - revokes key
- `POST /auth/keys/:id/rotate`
  - rotates secret, returns new `api_key`

Response model:

- `api_key` is returned only on create/rotate.
- list endpoint never returns secrets/hashes.

## Scope Model

Current route scope mapping:

- `auth:manage` -> `/auth/keys*`
- `devices:manage` -> `/devices*`, `/app/*` (device operations)
- `users:read` -> `/user/*`
- `chats:read` -> `/chats`, `/chat/*`, `/ws`
- `messages:send` -> `/send/*`
- `messages:manage` -> `/message/*` and chat mutation operations
- `groups:manage` -> `/group/*`
- `newsletters:manage` -> `/newsletter/*`
- `chatwoot:sync` -> `/chatwoot/sync*`

Special rules:

- Basic auth and shared bearer token have full access.
- Scope `*` grants full access for API-key-authenticated requests.

## Scope Reference Table

| Scope | Intended Access |
|---|---|
| `auth:manage` | Manage API keys (`/auth/keys*`) |
| `devices:manage` | Device lifecycle and app connection routes |
| `users:read` | User/account information routes |
| `chats:read` | Chat listing, chat messages, websocket feed |
| `messages:send` | Send message/media routes (`/send/*`) |
| `messages:manage` | Revoke/react/update/delete/read/download routes |
| `groups:manage` | Group admin and participant routes |
| `newsletters:manage` | Newsletter routes |
| `chatwoot:sync` | Chatwoot sync endpoints |

## Recommended Key Profiles

| Integration | Recommended Scopes |
|---|---|
| n8n sender bot | `messages:send`, `chats:read` |
| Backoffice moderation | `messages:manage`, `chats:read`, `users:read` |
| Device admin automation | `devices:manage`, `auth:manage` |
| Chatwoot sync worker | `chatwoot:sync`, `chats:read` |

## Security Notes

- Rotate keys regularly.
- Use short expirations for automation keys.
- Keep management scope (`auth:manage`) restricted to admin keys only.
- Prefer one key per integration/service to simplify revocation and auditing.

Hardening recommendations:

- Never share the same key across environments.
- Use one key per service identity (blast-radius reduction).
- Use `expires_in_days` whenever possible.
- Alert on unusual key usage patterns (`last_used_at` drift).

## Zero-Downtime Rotation Runbook

1. Call `POST /auth/keys/:id/rotate` and capture new key.
2. Deploy new key to client/integration secret store.
3. Validate requests with new key in production.
4. Remove old key secret from all clients.
5. Keep same key ID and scope set (already preserved by rotation).

## Incident Response Runbook

If key leak is suspected:

1. Immediately `DELETE /auth/keys/:id`.
2. Create replacement key with minimum required scopes.
3. Redeploy integration secrets.
4. Audit affected routes and usage interval using `last_used_at` and app logs.
5. Shorten expiration policy for future keys.

## Migration: Shared Token -> Scoped Keys

1. Keep current `APP_AUTH_TOKEN` active during migration window.
2. Create scoped keys for each integration.
3. Move integrations one by one to `X-API-Key`.
4. Verify all routes still authorized.
5. Remove shared token once migration completes.

## Example

Create admin key:

```bash
curl -X POST http://localhost:3000/auth/keys \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name":"n8n-prod","scopes":["messages:send","chats:read"],"expires_in_days":30}'
```

Use scoped key:

```bash
curl -X POST http://localhost:3000/send/message \
  -H "X-API-Key: gowa.<id>.<secret>" \
  -H "Content-Type: application/json" \
  -d '{"phone":"628123456789","message":"hello"}'
```

Scope failure example:

```bash
curl -X DELETE http://localhost:3000/devices/my-device \
  -H "X-API-Key: gowa.<id>.<secret-without-devices-scope>"
```

Expected:
- `403 FORBIDDEN_SCOPE`
