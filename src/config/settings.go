package config

import (
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

var (
	AppVersion             = "v8.3.0"
	AppPort                = "3000"
	AppHost                = "0.0.0.0"
	AppDebug               = false
	AppOs                  = "Chrome"
	AppPlatform            = waCompanionReg.DeviceProps_PlatformType(1)
	AppBasicAuthCredential []string
	AppAuthToken           = ""
	AppCorsOrigins         []string
	AppSecurityHeaders     = true
	AppRateLimitEnabled    = false
	AppRateLimitMax        = 120
	AppRateLimitWindowSec  = 60
	AppBasePath            = ""
	AppTrustedProxies      []string // Trusted proxy IP ranges (e.g., "0.0.0.0/0" for all, or specific CIDRs)

	McpPort = "8080"
	McpHost = "localhost"

	PathQrCode    = "statics/qrcode"
	PathSendItems = "statics/senditems"
	PathMedia     = "statics/media"
	PathStorages  = "storages"

	DBURI     = "file:storages/whatsapp.db?_foreign_keys=on"
	DBKeysURI = ""

	WhatsappAutoReplyMessage          string
	WhatsappAutoMarkRead              = false // Auto-mark incoming messages as read
	WhatsappAutoDownloadMedia         = true  // Auto-download media from incoming messages
	WhatsappAutoDownloadStatusMedia   = false // Auto-download status/story media from incoming events
	WhatsappHistorySyncDumpEnabled    = false // Persist raw WhatsApp history sync payload to disk (can be large/sensitive)
	WhatsappWebhook                   []string
	WhatsappWebhookSecret             = ""
	WhatsappWebhookInsecureSkipVerify = false          // Skip TLS certificate verification for webhooks (insecure)
	WhatsappWebhookEvents             []string         // Whitelist of events to forward to webhook (empty = all events)
	WhatsappAutoRejectCall                     = false // Auto-reject incoming calls
	WhatsappLogLevel                           = "ERROR"
	WhatsappSettingMaxImageSize       int64    = 20000000  // 20MB
	WhatsappSettingMaxFileSize        int64    = 50000000  // 50MB
	WhatsappSettingMaxVideoSize       int64    = 100000000 // 100MB
	WhatsappSettingMaxDownloadSize    int64    = 500000000 // 500MB
	WhatsappTypeUser                           = "@s.whatsapp.net"
	WhatsappTypeGroup                          = "@g.us"
	WhatsappTypeLid                            = "@lid"
	WhatsappAccountValidation                  = true
	WhatsappPresenceOnConnect                  = "unavailable" // Presence to send on connect: "available", "unavailable", or "none"

	ChatStorageURI               = "file:storages/chatstorage.db"
	ChatStorageEnableForeignKeys = true
	ChatStorageEnableWAL         = true

	ChatwootEnabled      = false
	ChatwootURL          = ""
	ChatwootAPIToken     = ""
	ChatwootWebhookToken = "" // Optional token to secure /chatwoot/webhook (header X-Chatwoot-Token or query token)
	ChatwootAccountID    = 0
	ChatwootInboxID      = 0
	ChatwootDeviceID     = "" // Device ID for outbound messages (required for multi-device)

	ChatWootSyncAvatar            = false // Sync WhatsApp profile picture to Chatwoot contacts
	ChatWootEnableTypingIndicator = false // Enable typing indicators in Chatwoot based on WhatsApp activity

	// Chatwoot History Sync settings
	ChatwootImportMessages                = false    // Enable message history import to Chatwoot
	ChatwootDaysLimitImportMessages       = 3        // Days of history to import (default: 3)
	ChatwootSyncIncludeMedia              = true     // Download media attachments during sync
	ChatwootSyncIncludeGroups             = true     // Include group chats during sync
	ChatwootSyncIncludeStatus             = false    // Include status/story chat in Chatwoot sync
	ChatwootSyncMaxMessagesPerChat        = 500      // Max messages to sync per chat
	ChatwootSyncBatchSize                 = 10       // Number of messages per batch before delay
	ChatwootSyncDelayMs                   = 500      // Delay between batches in milliseconds
	ChatwootSyncMaxMediaFileSize    int64 = 20000000 // Max media size to download during sync (20MB, 0 = unlimited)
)
