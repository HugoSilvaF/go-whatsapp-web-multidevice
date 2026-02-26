package cmd

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	domainChat "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chat"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainDevice "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/device"
	domainGroup "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/group"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	domainNewsletter "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/newsletter"
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/apikey"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/usecase"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.mau.fi/whatsmeow"
)

var (
	EmbedIndex embed.FS
	EmbedViews embed.FS

	// Whatsapp
	whatsappCli *whatsmeow.Client

	// Chat Storage
	chatStorageDB   *sql.DB
	chatStorageRepo domainChatStorage.IChatStorageRepository
	apiKeyService   *apikey.Service

	// Usecase
	appUsecase        domainApp.IAppUsecase
	chatUsecase       domainChat.IChatUsecase
	sendUsecase       domainSend.ISendUsecase
	userUsecase       domainUser.IUserUsecase
	messageUsecase    domainMessage.IMessageUsecase
	groupUsecase      domainGroup.IGroupUsecase
	newsletterUsecase domainNewsletter.INewsletterUsecase
	deviceUsecase     domainDevice.IDeviceUsecase
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Short: "Send free whatsapp API",
	Long: `This application is from clone https://github.com/aldinokemal/go-whatsapp-web-multidevice, 
you can send whatsapp over http api but your whatsapp account have to be multi device version`,
}

func init() {
	// Load environment variables first
	utils.LoadConfig(".")

	time.Local = time.UTC

	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Initialize flags first, before any subcommands are added
	initFlags()

	// Then initialize other components
	cobra.OnInitialize(initEnvConfig, initApp)
}

// initEnvConfig loads configuration from environment variables
func initEnvConfig() {
	// Application settings
	if envPort := viper.GetString("app_port"); envPort != "" {
		config.AppPort = envPort
	}
	if envHost := viper.GetString("app_host"); envHost != "" {
		config.AppHost = envHost
	}
	if envDebug := viper.GetBool("app_debug"); envDebug {
		config.AppDebug = envDebug
	}
	if envOs := viper.GetString("app_os"); envOs != "" {
		config.AppOs = envOs
	}
	if envBasicAuth := viper.GetString("app_basic_auth"); envBasicAuth != "" {
		credential := strings.Split(envBasicAuth, ",")
		config.AppBasicAuthCredential = credential
	}
	if envAuthToken := viper.GetString("app_auth_token"); envAuthToken != "" {
		config.AppAuthToken = envAuthToken
	}
	if viper.IsSet("app_security_headers") {
		config.AppSecurityHeaders = viper.GetBool("app_security_headers")
	}
	if viper.IsSet("app_rate_limit_enabled") {
		config.AppRateLimitEnabled = viper.GetBool("app_rate_limit_enabled")
	}
	if viper.IsSet("app_rate_limit_max") {
		config.AppRateLimitMax = viper.GetInt("app_rate_limit_max")
	}
	if viper.IsSet("app_rate_limit_window_sec") {
		config.AppRateLimitWindowSec = viper.GetInt("app_rate_limit_window_sec")
	}
	if envCorsOrigins := viper.GetString("app_cors_origins"); envCorsOrigins != "" {
		origins := strings.Split(envCorsOrigins, ",")
		config.AppCorsOrigins = origins
	}
	if envBasePath := viper.GetString("app_base_path"); envBasePath != "" {
		config.AppBasePath = envBasePath
	}
	if envTrustedProxies := viper.GetString("app_trusted_proxies"); envTrustedProxies != "" {
		proxies := strings.Split(envTrustedProxies, ",")
		config.AppTrustedProxies = proxies
	}

	// Database settings
	if envDBURI := viper.GetString("db_uri"); envDBURI != "" {
		config.DBURI = envDBURI
	}
	if envDBKEYSURI := viper.GetString("db_keys_uri"); envDBKEYSURI != "" {
		config.DBKeysURI = envDBKEYSURI
	}

	// WhatsApp settings
	if envAutoReply := viper.GetString("whatsapp_auto_reply"); envAutoReply != "" {
		config.WhatsappAutoReplyMessage = envAutoReply
	}
	if viper.IsSet("whatsapp_auto_mark_read") {
		config.WhatsappAutoMarkRead = viper.GetBool("whatsapp_auto_mark_read")
	}
	if viper.IsSet("whatsapp_auto_download_media") {
		config.WhatsappAutoDownloadMedia = viper.GetBool("whatsapp_auto_download_media")
	}
	if viper.IsSet("whatsapp_auto_download_status_media") {
		config.WhatsappAutoDownloadStatusMedia = viper.GetBool("whatsapp_auto_download_status_media")
	}
	if viper.IsSet("whatsapp_history_sync_dump_enabled") {
		config.WhatsappHistorySyncDumpEnabled = viper.GetBool("whatsapp_history_sync_dump_enabled")
	}
	if envWebhook := viper.GetString("whatsapp_webhook"); envWebhook != "" {
		webhook := strings.Split(envWebhook, ",")
		config.WhatsappWebhook = webhook
	}
	if envWebhookSecret := viper.GetString("whatsapp_webhook_secret"); envWebhookSecret != "" {
		config.WhatsappWebhookSecret = envWebhookSecret
	}
	if viper.IsSet("whatsapp_webhook_insecure_skip_verify") {
		config.WhatsappWebhookInsecureSkipVerify = viper.GetBool("whatsapp_webhook_insecure_skip_verify")
	}
	if envWebhookEvents := viper.GetString("whatsapp_webhook_events"); envWebhookEvents != "" {
		events := strings.Split(envWebhookEvents, ",")
		config.WhatsappWebhookEvents = events
	}
	if len(config.WhatsappWebhook) > 0 && strings.TrimSpace(config.WhatsappWebhookSecret) == "" {
		logrus.Fatalln("WHATSAPP_WEBHOOK_SECRET is required when WHATSAPP_WEBHOOK is configured")
	}
	if config.WhatsappWebhookInsecureSkipVerify {
		logrus.Warn("WHATSAPP_WEBHOOK_INSECURE_SKIP_VERIFY=true disables TLS verification; use only for development")
	}
	if viper.IsSet("whatsapp_account_validation") {
		config.WhatsappAccountValidation = viper.GetBool("whatsapp_account_validation")
	}
	if viper.IsSet("whatsapp_auto_reject_call") {
		config.WhatsappAutoRejectCall = viper.GetBool("whatsapp_auto_reject_call")
	}
	if envPresenceOnConnect := viper.GetString("whatsapp_presence_on_connect"); envPresenceOnConnect != "" {
		config.WhatsappPresenceOnConnect = envPresenceOnConnect
	}

	// Chatwoot settings
	if viper.IsSet("chatwoot_enabled") {
		config.ChatwootEnabled = viper.GetBool("chatwoot_enabled")
	}
	if envChatwootURL := viper.GetString("chatwoot_url"); envChatwootURL != "" {
		config.ChatwootURL = envChatwootURL
	}
	if envChatwootAPIToken := viper.GetString("chatwoot_api_token"); envChatwootAPIToken != "" {
		config.ChatwootAPIToken = envChatwootAPIToken
	}
	if envChatwootWebhookToken := viper.GetString("chatwoot_webhook_token"); envChatwootWebhookToken != "" {
		config.ChatwootWebhookToken = envChatwootWebhookToken
	}
	if viper.IsSet("chatwoot_account_id") {
		config.ChatwootAccountID = viper.GetInt("chatwoot_account_id")
	}
	if viper.IsSet("chatwoot_inbox_id") {
		config.ChatwootInboxID = viper.GetInt("chatwoot_inbox_id")
	}
	if envChatwootDeviceID := viper.GetString("chatwoot_device_id"); envChatwootDeviceID != "" {
		config.ChatwootDeviceID = envChatwootDeviceID
	}
	// Chatwoot History Sync settings
	if viper.IsSet("chatwoot_import_messages") {
		config.ChatwootImportMessages = viper.GetBool("chatwoot_import_messages")
	}
	if viper.IsSet("chatwoot_days_limit_import_messages") {
		config.ChatwootDaysLimitImportMessages = viper.GetInt("chatwoot_days_limit_import_messages")
	}
	if viper.IsSet("chatwoot_sync_include_media") {
		config.ChatwootSyncIncludeMedia = viper.GetBool("chatwoot_sync_include_media")
	}
	if viper.IsSet("chatwoot_sync_include_groups") {
		config.ChatwootSyncIncludeGroups = viper.GetBool("chatwoot_sync_include_groups")
	}
	if viper.IsSet("chatwoot_sync_include_status") {
		config.ChatwootSyncIncludeStatus = viper.GetBool("chatwoot_sync_include_status")
	}
	if viper.IsSet("chatwoot_sync_max_messages_per_chat") {
		config.ChatwootSyncMaxMessagesPerChat = viper.GetInt("chatwoot_sync_max_messages_per_chat")
	}
	if viper.IsSet("chatwoot_sync_batch_size") {
		config.ChatwootSyncBatchSize = viper.GetInt("chatwoot_sync_batch_size")
	}
	if viper.IsSet("chatwoot_sync_delay_ms") {
		config.ChatwootSyncDelayMs = viper.GetInt("chatwoot_sync_delay_ms")
	}
	if viper.IsSet("chatwoot_sync_max_media_file_size") {
		config.ChatwootSyncMaxMediaFileSize = viper.GetInt64("chatwoot_sync_max_media_file_size")
	}

	if viper.IsSet("chatwoot_sync_avatar") {
		config.ChatWootSyncAvatar = viper.GetBool("chatwoot_sync_avatar")
	}
	if viper.IsSet("chatwoot_enable_typing_indicator") {
		config.ChatWootEnableTypingIndicator = viper.GetBool("chatwoot_enable_typing_indicator")
	}
}

func initFlags() {
	// Application flags
	rootCmd.PersistentFlags().StringVarP(
		&config.AppPort,
		"port", "p",
		config.AppPort,
		"change port number with --port <number> | example: --port=8080",
	)

	rootCmd.PersistentFlags().StringVarP(
		&config.AppHost,
		"host", "H",
		config.AppHost,
		`host to bind the server --host <string> | example: --host="127.0.0.1"`,
	)

	rootCmd.PersistentFlags().BoolVarP(
		&config.AppDebug,
		"debug", "d",
		config.AppDebug,
		"hide or displaying log with --debug <true/false> | example: --debug=true",
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.AppOs,
		"os", "",
		config.AppOs,
		`os name --os <string> | example: --os="Chrome"`,
	)
	rootCmd.PersistentFlags().StringSliceVarP(
		&config.AppBasicAuthCredential,
		"basic-auth", "b",
		config.AppBasicAuthCredential,
		"basic auth credential | -b=yourUsername:yourPassword",
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.AppAuthToken,
		"auth-token", "",
		config.AppAuthToken,
		`single shared token for API authentication (Authorization: Bearer <token> or X-API-Key) --auth-token <string> | example: --auth-token="super-secret-token"`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.AppSecurityHeaders,
		"security-headers", "",
		config.AppSecurityHeaders,
		`enable secure HTTP response headers --security-headers <true/false> | example: --security-headers=true`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.AppRateLimitEnabled,
		"rate-limit-enabled", "",
		config.AppRateLimitEnabled,
		`enable global request rate limiting by client IP --rate-limit-enabled <true/false> | example: --rate-limit-enabled=true`,
	)
	rootCmd.PersistentFlags().IntVarP(
		&config.AppRateLimitMax,
		"rate-limit-max", "",
		config.AppRateLimitMax,
		`max requests allowed per window for rate limiter --rate-limit-max <int> | example: --rate-limit-max=120`,
	)
	rootCmd.PersistentFlags().IntVarP(
		&config.AppRateLimitWindowSec,
		"rate-limit-window-sec", "",
		config.AppRateLimitWindowSec,
		`rate limiter window in seconds --rate-limit-window-sec <int> | example: --rate-limit-window-sec=60`,
	)
	rootCmd.PersistentFlags().StringSliceVarP(
		&config.AppCorsOrigins,
		"cors-origins", "",
		config.AppCorsOrigins,
		`allowed CORS origins (empty disables CORS middleware) --cors-origins <string> | example: --cors-origins="https://app.example.com,http://localhost:5173"`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.AppBasePath,
		"base-path", "",
		config.AppBasePath,
		`base path for subpath deployment --base-path <string> | example: --base-path="/gowa"`,
	)
	rootCmd.PersistentFlags().StringSliceVarP(
		&config.AppTrustedProxies,
		"trusted-proxies", "",
		config.AppTrustedProxies,
		`trusted proxy IP ranges for reverse proxy deployments --trusted-proxies <string> | example: --trusted-proxies="0.0.0.0/0" or --trusted-proxies="10.0.0.0/8,172.16.0.0/12"`,
	)

	// Database flags
	rootCmd.PersistentFlags().StringVarP(
		&config.DBURI,
		"db-uri", "",
		config.DBURI,
		`the database uri to store the connection data database uri (by default, we'll use sqlite3 under storages/whatsapp.db). database uri --db-uri <string> | example: --db-uri="file:storages/whatsapp.db?_foreign_keys=on or postgres://user:password@localhost:5432/whatsapp"`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.DBKeysURI,
		"db-keys-uri", "",
		config.DBKeysURI,
		`the database uri to store the keys database uri (by default, we'll use the same database uri). database uri --db-keys-uri <string> | example: --db-keys-uri="file::memory:?cache=shared&_foreign_keys=on"`,
	)

	// WhatsApp flags
	rootCmd.PersistentFlags().StringVarP(
		&config.WhatsappAutoReplyMessage,
		"autoreply", "",
		config.WhatsappAutoReplyMessage,
		`auto reply when received message --autoreply <string> | example: --autoreply="Don't reply this message"`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappAutoMarkRead,
		"auto-mark-read", "",
		config.WhatsappAutoMarkRead,
		`auto mark incoming messages as read --auto-mark-read <true/false> | example: --auto-mark-read=true`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappAutoDownloadMedia,
		"auto-download-media", "",
		config.WhatsappAutoDownloadMedia,
		`auto download media from incoming messages --auto-download-media <true/false> | example: --auto-download-media=false`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappAutoDownloadStatusMedia,
		"auto-download-status-media", "",
		config.WhatsappAutoDownloadStatusMedia,
		`auto download status/story media from incoming events --auto-download-status-media <true/false> | example: --auto-download-status-media=false`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappHistorySyncDumpEnabled,
		"history-sync-dump-enabled", "",
		config.WhatsappHistorySyncDumpEnabled,
		`persist raw history sync payloads to files (may contain sensitive data and large payloads) --history-sync-dump-enabled <true/false> | example: --history-sync-dump-enabled=false`,
	)
	rootCmd.PersistentFlags().StringSliceVarP(
		&config.WhatsappWebhook,
		"webhook", "w",
		config.WhatsappWebhook,
		`forward event to webhook --webhook <string> | example: --webhook="https://yourcallback.com/callback"`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.WhatsappWebhookSecret,
		"webhook-secret", "",
		config.WhatsappWebhookSecret,
		`secure webhook request --webhook-secret <string> | example: --webhook-secret="super-secret-key"`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappWebhookInsecureSkipVerify,
		"webhook-insecure-skip-verify", "",
		config.WhatsappWebhookInsecureSkipVerify,
		`skip TLS certificate verification for webhooks (INSECURE - use only for development/self-signed certs) --webhook-insecure-skip-verify <true/false> | example: --webhook-insecure-skip-verify=true`,
	)
	rootCmd.PersistentFlags().StringSliceVarP(
		&config.WhatsappWebhookEvents,
		"webhook-events", "",
		config.WhatsappWebhookEvents,
		`whitelist of events to forward to webhook (empty = all events) --webhook-events <string> | example: --webhook-events="message,message.ack,group.participants"`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappAccountValidation,
		"account-validation", "",
		config.WhatsappAccountValidation,
		`enable or disable account validation --account-validation <true/false> | example: --account-validation=true`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappAutoRejectCall,
		"auto-reject-call", "",
		config.WhatsappAutoRejectCall,
		`auto reject incoming calls --auto-reject-call <true/false> | example: --auto-reject-call=true`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.WhatsappPresenceOnConnect,
		"presence-on-connect", "",
		config.WhatsappPresenceOnConnect,
		`presence to send on connect: "available", "unavailable", or "none" --presence-on-connect <string> | example: --presence-on-connect="unavailable"`,
	)

	// Chatwoot flags
	rootCmd.PersistentFlags().BoolVarP(
		&config.ChatwootEnabled,
		"chatwoot-enabled", "",
		config.ChatwootEnabled,
		`enable Chatwoot integration --chatwoot-enabled <true/false> | example: --chatwoot-enabled=true`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.ChatwootDeviceID,
		"chatwoot-device-id", "",
		config.ChatwootDeviceID,
		`device ID for Chatwoot outbound messages --chatwoot-device-id <string> | example: --chatwoot-device-id="my-device"`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.ChatwootWebhookToken,
		"chatwoot-webhook-token", "",
		config.ChatwootWebhookToken,
		`optional shared token for /chatwoot/webhook (header X-Chatwoot-Token or query token) --chatwoot-webhook-token <string> | example: --chatwoot-webhook-token="cw-secret"`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.ChatwootImportMessages,
		"chatwoot-import-messages", "",
		config.ChatwootImportMessages,
		`enable message history import to Chatwoot --chatwoot-import-messages <true/false> | example: --chatwoot-import-messages=true`,
	)
	rootCmd.PersistentFlags().IntVarP(
		&config.ChatwootDaysLimitImportMessages,
		"chatwoot-days-limit-import-messages", "",
		config.ChatwootDaysLimitImportMessages,
		`days of message history to import to Chatwoot --chatwoot-days-limit-import-messages <int> | example: --chatwoot-days-limit-import-messages=7`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.ChatwootSyncIncludeMedia,
		"chatwoot-sync-include-media", "",
		config.ChatwootSyncIncludeMedia,
		`include media attachments in Chatwoot sync --chatwoot-sync-include-media <true/false> | example: --chatwoot-sync-include-media=true`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.ChatwootSyncIncludeGroups,
		"chatwoot-sync-include-groups", "",
		config.ChatwootSyncIncludeGroups,
		`include group chats in Chatwoot sync --chatwoot-sync-include-groups <true/false> | example: --chatwoot-sync-include-groups=true`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.ChatwootSyncIncludeStatus,
		"chatwoot-sync-include-status", "",
		config.ChatwootSyncIncludeStatus,
		`include status/story chat in Chatwoot sync --chatwoot-sync-include-status <true/false> | example: --chatwoot-sync-include-status=false`,
	)
	rootCmd.PersistentFlags().IntVarP(
		&config.ChatwootSyncMaxMessagesPerChat,
		"chatwoot-sync-max-messages-per-chat", "",
		config.ChatwootSyncMaxMessagesPerChat,
		`max messages per chat in Chatwoot sync --chatwoot-sync-max-messages-per-chat <int> | example: --chatwoot-sync-max-messages-per-chat=300`,
	)
	rootCmd.PersistentFlags().IntVarP(
		&config.ChatwootSyncBatchSize,
		"chatwoot-sync-batch-size", "",
		config.ChatwootSyncBatchSize,
		`batch size for Chatwoot sync throttling --chatwoot-sync-batch-size <int> | example: --chatwoot-sync-batch-size=10`,
	)
	rootCmd.PersistentFlags().IntVarP(
		&config.ChatwootSyncDelayMs,
		"chatwoot-sync-delay-ms", "",
		config.ChatwootSyncDelayMs,
		`delay between Chatwoot sync batches in ms --chatwoot-sync-delay-ms <int> | example: --chatwoot-sync-delay-ms=500`,
	)
	rootCmd.PersistentFlags().Int64VarP(
		&config.ChatwootSyncMaxMediaFileSize,
		"chatwoot-sync-max-media-file-size", "",
		config.ChatwootSyncMaxMediaFileSize,
		`max media file size (bytes) to download during Chatwoot sync (0 = unlimited) --chatwoot-sync-max-media-file-size <int> | example: --chatwoot-sync-max-media-file-size=20000000`,
	)
}

func initChatStorage() (*sql.DB, error) {
	connStr := config.ChatStorageURI
	separator := "?"
	if strings.Contains(connStr, "?") {
		separator = "&"
	}
	connStr = fmt.Sprintf("%s%s_journal_mode=WAL", connStr, separator)
	if config.ChatStorageEnableForeignKeys {
		connStr += "&_foreign_keys=on"
	}

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func initApp() {
	if config.AppDebug {
		config.WhatsappLogLevel = "DEBUG"
		logrus.SetLevel(logrus.DebugLevel)
	}

	//preparing folder if not exist
	err := utils.CreateFolder(config.PathQrCode, config.PathSendItems, config.PathStorages, config.PathMedia)
	if err != nil {
		logrus.Errorln(err)
	}

	ctx := context.Background()

	chatStorageDB, err = initChatStorage()
	if err != nil {
		// Terminate the application if chat storage fails to initialize to avoid nil pointer panics later.
		logrus.Fatalf("failed to initialize chat storage: %v", err)
	}

	chatStorageRepo = chatstorage.NewStorageRepository(chatStorageDB)
	chatStorageRepo.InitializeSchema()
	apiKeyService = apikey.NewService(chatStorageDB)
	if err := apiKeyService.InitializeSchema(); err != nil {
		logrus.Fatalf("failed to initialize api key schema: %v", err)
	}

	whatsappDB := whatsapp.InitWaDB(ctx, config.DBURI)
	var keysDB *sqlstore.Container
	if config.DBKeysURI != "" {
		keysDB = whatsapp.InitWaDB(ctx, config.DBKeysURI)
	}

	whatsappCli = whatsapp.InitWaCLI(ctx, whatsappDB, keysDB, chatStorageRepo)

	// Initialize device manager and usecase for multi-device support
	dm := whatsapp.GetDeviceManager()
	if dm != nil {
		_ = dm.LoadExistingDevices(ctx)
	}

	// Usecase
	appUsecase = usecase.NewAppService(chatStorageRepo, dm)
	chatUsecase = usecase.NewChatService(chatStorageRepo)
	sendUsecase = usecase.NewSendService(appUsecase, chatStorageRepo)
	userUsecase = usecase.NewUserService()
	messageUsecase = usecase.NewMessageService(chatStorageRepo)
	groupUsecase = usecase.NewGroupService()
	newsletterUsecase = usecase.NewNewsletterService()
	deviceUsecase = usecase.NewDeviceService(dm)
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute(embedIndex embed.FS, embedViews embed.FS) {
	EmbedIndex = embedIndex
	EmbedViews = embedViews
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
