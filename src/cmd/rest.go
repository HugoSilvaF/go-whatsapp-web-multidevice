package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatwoot"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/helpers"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/middleware"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/websocket"
	"github.com/dustin/go-humanize"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var restCmd = &cobra.Command{
	Use:   "rest",
	Short: "Send whatsapp API over http",
	Long:  `This application is from clone https://github.com/aldinokemal/go-whatsapp-web-multidevice`,
	Run:   restServer,
}

func init() {
	rootCmd.AddCommand(restCmd)
}
func restServer(_ *cobra.Command, _ []string) {
	engine := html.NewFileSystem(http.FS(EmbedIndex), ".html")
	engine.AddFunc("isEnableBasicAuth", func(token any) bool {
		return token != nil
	})
	fiberConfig := fiber.Config{
		Views:                   engine,
		EnableTrustedProxyCheck: true,
		BodyLimit:               int(config.WhatsappSettingMaxVideoSize),
		Network:                 "tcp",
	}

	// Configure proxy settings if trusted proxies are specified
	if len(config.AppTrustedProxies) > 0 {
		fiberConfig.TrustedProxies = config.AppTrustedProxies
		fiberConfig.ProxyHeader = fiber.HeaderXForwardedHost
	}

	app := fiber.New(fiberConfig)

	app.Static(config.AppBasePath+"/statics", "./statics")
	app.Use(config.AppBasePath+"/components", filesystem.New(filesystem.Config{
		Root:       http.FS(EmbedViews),
		PathPrefix: "views/components",
		Browse:     true,
	}))
	app.Use(config.AppBasePath+"/assets", filesystem.New(filesystem.Config{
		Root:       http.FS(EmbedViews),
		PathPrefix: "views/assets",
		Browse:     true,
	}))

	app.Use(middleware.Recovery())
	app.Use(middleware.RequestTimeout(middleware.DefaultRequestTimeout))
	if config.AppSecurityHeaders {
		app.Use(middleware.SecurityHeaders())
	}
	if config.AppRateLimitEnabled {
		app.Use(middleware.RequestRateLimit(config.AppRateLimitMax, config.AppRateLimitWindowSec, config.AppBasePath))
	}
	app.Use(middleware.BasicAuth())
	if config.AppDebug {
		app.Use(logger.New())
	}
	if len(config.AppCorsOrigins) > 0 {
		allowOrigins := strings.Join(config.AppCorsOrigins, ",")
		if allowOrigins == "*" && (len(config.AppBasicAuthCredential) > 0 || config.AppAuthToken != "") {
			logrus.Warn("CORS is configured with '*' while authentication is enabled; restrict APP_CORS_ORIGINS in production")
		}
		app.Use(cors.New(cors.Config{
			AllowOrigins: allowOrigins,
			AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Device-Id, X-API-Key, X-Chatwoot-Token",
		}))
	}

	// Device manager - needed for chatwoot webhook
	dm := whatsapp.GetDeviceManager()

	// Chatwoot webhook - registered BEFORE basic auth middleware
	// This allows Chatwoot to send webhooks without authentication
	if config.ChatwootEnabled {
		// Initialize global sync service early so event handlers can use avatar sync
		// even before /chatwoot/sync endpoint is called.
		chatwoot.GetSyncService(chatwoot.GetDefaultClient(), chatStorageRepo)

		chatwootHandler := rest.NewChatwootHandler(appUsecase, sendUsecase, dm, chatStorageRepo)
		webhookPath := "/chatwoot/webhook"
		if config.AppBasePath != "" {
			webhookPath = config.AppBasePath + webhookPath
		}
		app.Post(webhookPath, chatwootHandler.HandleWebhook)
	}

	if len(config.AppBasicAuthCredential) > 0 {
		logrus.Infof("Basic authentication enabled with %d credential(s)", len(config.AppBasicAuthCredential))
	}
	if config.AppAuthToken != "" {
		logrus.Info("Token authentication enabled (Authorization: Bearer <token> or X-API-Key)")
	}

	// Public health endpoint for load balancers/orchestrators.
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "go-whatsapp-web-multidevice",
			"version":     config.AppVersion,
			"server_time": time.Now().UTC().Format(time.RFC3339),
		})
	})

	var keyValidator middleware.APIKeyValidator
	if apiKeyService != nil {
		keyValidator = func(ctx context.Context, rawKey string) (*middleware.APIKeyValidationResult, error) {
			res, err := apiKeyService.Authenticate(ctx, rawKey)
			if err != nil {
				return nil, err
			}
			return &middleware.APIKeyValidationResult{
				KeyID:  res.KeyID,
				Scopes: res.Scopes,
			}, nil
		}
	}

	account := make(map[string]string)
	for _, basicAuth := range config.AppBasicAuthCredential {
		ba := strings.SplitN(basicAuth, ":", 2)
		if len(ba) != 2 {
			logrus.Fatalln("Basic auth is not valid, please this following format <user>:<secret>")
		}
		account[ba[0]] = ba[1]
	}
	app.Use(middleware.RequireAuth(account, config.AppAuthToken, keyValidator))

	// Create base path group or use app directly
	var apiGroup fiber.Router = app
	if config.AppBasePath != "" {
		apiGroup = app.Group(config.AppBasePath)
	}

	registerDeviceScopedRoutes := func(r fiber.Router) {
		rest.InitRestApp(r.Group("", middleware.RequireScope("devices:manage")), appUsecase)
		rest.InitRestChat(r.Group("", middleware.RequireScope("chats:read", "messages:manage")), chatUsecase)
		rest.InitRestSend(r.Group("", middleware.RequireScope("messages:send")), sendUsecase)
		rest.InitRestUser(r.Group("", middleware.RequireScope("users:read")), userUsecase)
		rest.InitRestMessage(r.Group("", middleware.RequireScope("messages:manage")), messageUsecase)
		rest.InitRestGroup(r.Group("", middleware.RequireScope("groups:manage")), groupUsecase)
		rest.InitRestNewsletter(r.Group("", middleware.RequireScope("newsletters:manage")), newsletterUsecase)
		websocket.RegisterRoutes(r.Group("", middleware.RequireScope("chats:read")), appUsecase)
	}

	// Device management routes (no device_id required)
	rest.InitRestDevice(apiGroup.Group("", middleware.RequireScope("devices:manage")), deviceUsecase)

	// API key management routes
	if apiKeyService != nil {
		rest.InitRestAuth(apiGroup.Group("", middleware.RequireScope("auth:manage")), apiKeyService)
	}

	// Device-scoped operations (header-based)
	headerDeviceGroup := apiGroup.Group("", middleware.DeviceMiddleware(dm))
	registerDeviceScopedRoutes(headerDeviceGroup)

	// Chatwoot sync routes - require authentication (webhook is registered earlier without auth)
	if config.ChatwootEnabled {
		chatwootHandler := rest.NewChatwootHandler(appUsecase, sendUsecase, dm, chatStorageRepo)
		chatwootSyncGroup := apiGroup.Group("", middleware.RequireScope("chatwoot:sync"))
		chatwootSyncGroup.Post("/chatwoot/sync", chatwootHandler.SyncHistory)
		chatwootSyncGroup.Get("/chatwoot/sync/status", chatwootHandler.SyncStatus)
	}

	apiGroup.Get("/", func(c *fiber.Ctx) error {
		return c.Render("views/index", fiber.Map{
			"AppHost":        fmt.Sprintf("%s://%s", c.Protocol(), c.Hostname()),
			"AppVersion":     config.AppVersion,
			"AppBasePath":    config.AppBasePath,
			"BasicAuthToken": c.UserContext().Value(middleware.AuthorizationValue("BASIC_AUTH")),
			"MaxFileSize":    humanize.Bytes(uint64(config.WhatsappSettingMaxFileSize)),
			"MaxVideoSize":   humanize.Bytes(uint64(config.WhatsappSettingMaxVideoSize)),
		})
	})

	go websocket.RunHub()

	// Set auto reconnect to whatsapp server after booting
	go helpers.SetAutoConnectAfterBooting(appUsecase)

	// Set auto reconnect checking with a guaranteed client instance
	startAutoReconnectCheckerIfClientAvailable()

	if err := app.Listen(config.AppHost + ":" + config.AppPort); err != nil {
		logrus.Fatalln("Failed to start: ", err.Error())
	}
}
