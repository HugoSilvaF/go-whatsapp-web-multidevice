# Route Reference

This reference complements [`docs/openapi.yaml`](./openapi.yaml) with a practical route map: required parameters, success response, and common errors.

## Authentication and Headers

- API auth (optional, but recommended):
  - `Authorization: Basic <base64(user:pass)>` when `APP_BASIC_AUTH` is configured
  - `Authorization: Bearer <token>` or `X-API-Key: <token>` when `APP_AUTH_TOKEN` is configured
- API keys (scoped):
  - Managed via `/auth/keys` endpoints.
  - Key format: `gowa.<id>.<secret>`
  - Scopes currently used by routes:
    - `auth:manage`
    - `devices:manage`
    - `users:read`
    - `chats:read`
    - `messages:send`
    - `messages:manage`
    - `groups:manage`
    - `newsletters:manage`
    - `chatwoot:sync`
- Device scoping:
  - Use `X-Device-Id` or query `device_id` for device-scoped routes
  - If only one device exists, server can auto-resolve it
- Chatwoot webhook hardening:
  - `X-Chatwoot-Token` or query `token` when `CHATWOOT_WEBHOOK_TOKEN` is configured

## Response Pattern

- Success:
  - `200 OK` with payload `{ code, message, results }`
- Common errors:
  - `400` invalid params/body/validation
  - `401` unauthorized (`UNAUTHORIZED`)
  - `429` rate-limited (`RATE_LIMITED`) when enabled
  - `404` device/chat/message not found
  - `409` conflict (e.g., sync already running)
  - `500` internal server error
  - `503` service/device manager unavailable

## Device Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| GET | `/devices` | - | `DeviceListResponse` | `500` |
| POST | `/devices` | body optional `device_id` | `DeviceAddResponse` | `400`, `500` |
| GET | `/devices/:device_id` | path `device_id` | `DeviceInfoResponse` | `404`, `500` |
| DELETE | `/devices/:device_id` | path `device_id` | `GenericResponse` | `404`, `500` |
| GET | `/devices/:device_id/login` | path `device_id` | `LoginResponse` | `404`, `500` |
| POST | `/devices/:device_id/login/code` | path `device_id`, query `phone` | `LoginWithCodeResponse` | `400`, `404`, `500` |
| POST | `/devices/:device_id/logout` | path `device_id` | `GenericResponse` | `404`, `500` |
| POST | `/devices/:device_id/reconnect` | path `device_id` | `GenericResponse` | `404`, `500` |
| GET | `/devices/:device_id/status` | path `device_id` | `DeviceStatusResponse` | `404`, `500` |

## App Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| GET | `/healthz` | - | `{status, service, version, server_time}` | `500` |
| GET | `/app/login` | `X-Device-Id`/`device_id` | `LoginResponse` | `400`, `404`, `500` |
| GET | `/app/login-with-code` | `X-Device-Id`/`device_id`, query `phone` | `LoginWithCodeResponse` | `400`, `404`, `500` |
| GET | `/app/logout` | `X-Device-Id`/`device_id` | `GenericResponse` | `400`, `404`, `500` |
| GET | `/app/reconnect` | `X-Device-Id`/`device_id` | `GenericResponse` | `400`, `404`, `500` |
| GET | `/app/devices` | - | `DeviceResponse` | `500` |
| GET | `/app/status` | `X-Device-Id`/`device_id` | status object (`is_connected`, `is_logged_in`) | `400`, `404`, `500` |

## User Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| GET | `/user/info` | `X-Device-Id`/`device_id`, query `phone` | `UserInfoResponse` | `400`, `404`, `500` |
| GET | `/user/avatar` | `X-Device-Id`/`device_id`, query `phone` | `UserAvatarResponse` | `400`, `404`, `500` |
| POST | `/user/avatar` | `X-Device-Id`/`device_id`, multipart `avatar` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/user/pushname` | `X-Device-Id`/`device_id`, body `push_name` | `GenericResponse` | `400`, `404`, `500` |
| GET | `/user/my/privacy` | `X-Device-Id`/`device_id` | `UserPrivacyResponse` | `404`, `500` |
| GET | `/user/my/groups` | `X-Device-Id`/`device_id` | `GroupResponse` | `404`, `500` |
| GET | `/user/my/newsletters` | `X-Device-Id`/`device_id` | `NewsletterResponse` | `404`, `500` |
| GET | `/user/my/contacts` | `X-Device-Id`/`device_id` | `MyListContactsResponse` | `404`, `500` |
| GET | `/user/check` | `X-Device-Id`/`device_id`, query `phone` | `UserCheckResponse` | `400`, `404`, `500` |
| GET | `/user/business-profile` | `X-Device-Id`/`device_id`, query `phone` | `BusinessProfileResponse` | `400`, `404`, `500` |

## Send Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| POST | `/send/message` | `X-Device-Id`/`device_id`, body `phone`, `message` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/image` | `X-Device-Id`/`device_id`, `phone` + `image`/`image_url` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/file` | `X-Device-Id`/`device_id`, `phone` + `file`/`file_url` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/video` | `X-Device-Id`/`device_id`, `phone` + `video`/`video_url` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/sticker` | `X-Device-Id`/`device_id`, `phone` + `sticker`/`sticker_url` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/contact` | `X-Device-Id`/`device_id`, body `phone`, `contact_name`, `contact_phone` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/link` | `X-Device-Id`/`device_id`, body `phone`, `link_url` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/location` | `X-Device-Id`/`device_id`, body `phone`, `latitude`, `longitude` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/audio` | `X-Device-Id`/`device_id`, `phone` + `audio`/`audio_url` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/poll` | `X-Device-Id`/`device_id`, body `phone`, `name`, `options` | `SendMessageResponse` | `400`, `404`, `500` |
| POST | `/send/presence` | `X-Device-Id`/`device_id`, body `phone`, `presence` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/send/chat-presence` | `X-Device-Id`/`device_id`, body `phone`, `chat_presence` | `GenericResponse` | `400`, `404`, `500` |

## Message Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| POST | `/message/:message_id/reaction` | path `message_id`, body `phone`, `emoji` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/message/:message_id/revoke` | path `message_id`, body `phone` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/message/:message_id/delete` | path `message_id`, body `phone` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/message/:message_id/update` | path `message_id`, body `phone`, `message` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/message/:message_id/read` | path `message_id`, body `phone` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/message/:message_id/star` | path `message_id`, body `phone` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/message/:message_id/unstar` | path `message_id`, body `phone` | `GenericResponse` | `400`, `404`, `500` |
| GET | `/message/:message_id/download` | path `message_id`, query `phone` | media download response | `400`, `404`, `500` |

## Chat Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| GET | `/chats` | `X-Device-Id`/`device_id`, optional paging/query filters | `ChatListResponse` | `400`, `404`, `500` |
| GET | `/chat/:chat_jid/messages` | path `chat_jid`, optional paging query | `ChatMessagesResponse` | `400`, `404`, `500` |
| POST | `/chat/:chat_jid/pin` | path `chat_jid`, body `pinned` | `PinChatResponse` | `400`, `404`, `500` |
| POST | `/chat/:chat_jid/disappearing` | path `chat_jid`, body `timer_seconds` | `SetDisappearingTimerResponse` | `400`, `404`, `500` |
| POST | `/chat/:chat_jid/archive` | path `chat_jid`, body `archived` | `ArchiveChatResponse` | `400`, `404`, `500` |

## Group Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| POST | `/group` | body `name`, `participants[]` | `CreateGroupResponse` | `400`, `404`, `500` |
| POST | `/group/join-with-link` | body `code`/`link` | `GenericResponse` | `400`, `404`, `500` |
| GET | `/group/info-from-link` | query `link` | `GroupInfoFromLinkResponse` | `400`, `404`, `500` |
| GET | `/group/info` | query `group_id` | `GroupInfoResponse` | `400`, `404`, `500` |
| POST | `/group/leave` | body `group_id` | `GenericResponse` | `400`, `404`, `500` |
| GET | `/group/participants` | query `group_id` | `GroupResponse` | `400`, `404`, `500` |
| GET | `/group/participants/export` | query `group_id`, optional `format` | file export | `400`, `404`, `500` |
| POST | `/group/participants` | body `group_id`, `participants[]` | `ManageParticipantResponse` | `400`, `404`, `500` |
| POST | `/group/participants/remove` | body `group_id`, `participants[]` | `ManageParticipantResponse` | `400`, `404`, `500` |
| POST | `/group/participants/promote` | body `group_id`, `participants[]` | `ManageParticipantResponse` | `400`, `404`, `500` |
| POST | `/group/participants/demote` | body `group_id`, `participants[]` | `ManageParticipantResponse` | `400`, `404`, `500` |
| GET | `/group/participant-requests` | query `group_id` | `GroupParticipantRequestListResponse` | `400`, `404`, `500` |
| POST | `/group/participant-requests/approve` | body `group_id`, `participants[]` | `ManageParticipantResponse` | `400`, `404`, `500` |
| POST | `/group/participant-requests/reject` | body `group_id`, `participants[]` | `ManageParticipantResponse` | `400`, `404`, `500` |
| POST | `/group/photo` | multipart `group_id`, `photo` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/group/name` | body `group_id`, `name` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/group/locked` | body `group_id`, `locked` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/group/announce` | body `group_id`, `announce` | `GenericResponse` | `400`, `404`, `500` |
| POST | `/group/topic` | body `group_id`, `topic` | `GenericResponse` | `400`, `404`, `500` |
| GET | `/group/invite-link` | query `group_id` | `GetGroupInviteLinkResponse` | `400`, `404`, `500` |

## Newsletter Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| POST | `/newsletter/unfollow` | body `newsletter_id` | `GenericResponse` | `400`, `404`, `500` |

## Chatwoot Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| POST | `/chatwoot/sync` | body/query: `device_id`, `days`, `media`, `groups`, `status` | `ChatwootSyncResponse` | `400`, `401`, `404`, `409`, `500` |
| GET | `/chatwoot/sync/status` | query `device_id` | `ChatwootSyncStatusResponse` | `400`, `401`, `404`, `500` |
| POST | `/chatwoot/webhook` | payload from Chatwoot; token when configured | `200` empty body | `401`, `503` |

## Auth Routes

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| GET | `/auth/keys` | optional query `include_revoked=true|false` | list of key metadata (without secret/hash) | `400`, `401`, `403`, `500` |
| POST | `/auth/keys` | body `name`, `scopes[]`, optional `expires_in_days` | created key metadata + `api_key` (shown once) | `400`, `401`, `403`, `500` |
| DELETE | `/auth/keys/:id` | path `id` | `GenericResponse` | `401`, `403`, `404`, `500` |
| POST | `/auth/keys/:id/rotate` | path `id` | key metadata + new `api_key` (shown once) | `400`, `401`, `403`, `404`, `500` |

### Auth Route Contracts (Detailed)

`POST /auth/keys`
- Required body:
  - `name` (string, non-empty)
  - `scopes` (string array, at least 1)
- Optional body:
  - `expires_in_days` (number > 0)
- `200` response:
  - `results.id`
  - `results.name`
  - `results.scopes`
  - `results.expires_at` (nullable)
  - `results.created_at`
  - `results.api_key` (returned once, not recoverable later)
- Error codes:
  - `400 INVALID_REQUEST` missing/invalid fields
  - `401 UNAUTHORIZED` missing/invalid auth
  - `403 FORBIDDEN_SCOPE` authenticated key lacks `auth:manage`
  - `503 API_KEY_SERVICE_UNAVAILABLE` auth key service unavailable

`GET /auth/keys`
- Optional query:
  - `include_revoked` (`true|false`, default `false`)
- `200` response:
  - array of key metadata (never includes plaintext key or hash)
- Error codes:
  - `401 UNAUTHORIZED`
  - `403 FORBIDDEN_SCOPE`
  - `503 API_KEY_SERVICE_UNAVAILABLE`

`POST /auth/keys/:id/rotate`
- Required path:
  - `id` (key ID)
- `200` response:
  - key metadata + fresh `api_key` (shown once)
- Error codes:
  - `404 KEY_NOT_FOUND`
  - `400 INVALID_REQUEST` (for invalid operation, including revoked key rotation attempts)
  - `401 UNAUTHORIZED`
  - `403 FORBIDDEN_SCOPE`
  - `503 API_KEY_SERVICE_UNAVAILABLE`

`DELETE /auth/keys/:id`
- Required path:
  - `id` (key ID)
- `200` response:
  - generic success (`API key revoked`)
- Error codes:
  - `404 KEY_NOT_FOUND`
  - `401 UNAUTHORIZED`
  - `403 FORBIDDEN_SCOPE`
  - `503 API_KEY_SERVICE_UNAVAILABLE`

## WebSocket Route

| Method | Path | Required params | Success response | Common errors |
|---|---|---|---|---|
| GET (WS) | `/ws` | query `device_id` (when multiple devices) | websocket stream | handshake failures if unauthorized/invalid device |

## Chatwoot Sync Contracts (Detailed)

`POST /chatwoot/sync`
- Required:
  - device context (`X-Device-Id` header or `device_id` query/body where applicable)
- Optional payload/query tuning:
  - `days` (history depth, integer)
  - `media` (boolean)
  - `groups` (boolean)
  - `status` (boolean; include story/status chat)
- `200` response:
  - sync accepted/progress object with totals and counters
- Error codes:
  - `400 INVALID_REQUEST` invalid params
  - `401 UNAUTHORIZED`
  - `403 FORBIDDEN_SCOPE` without `chatwoot:sync`
  - `404` device not found/not resolved
  - `409` sync already running for this device
  - `500` unexpected sync failure

`GET /chatwoot/sync/status`
- Required:
  - device context
- `200` response:
  - current status/progress for selected device
- Error codes:
  - `400 INVALID_REQUEST`
  - `401 UNAUTHORIZED`
  - `403 FORBIDDEN_SCOPE`
  - `404` no device/status context
  - `500` internal error
