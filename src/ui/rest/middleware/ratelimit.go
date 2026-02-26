package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

func RequestRateLimit(max int, windowSeconds int, basePath string) fiber.Handler {
	if max <= 0 {
		max = 120
	}
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	webhookPath := strings.TrimRight(basePath, "/") + "/chatwoot/webhook"
	if webhookPath == "/chatwoot/webhook" || webhookPath == "//chatwoot/webhook" {
		webhookPath = "/chatwoot/webhook"
	}

	return limiter.New(limiter.Config{
		Max:                    max,
		Expiration:             time.Duration(windowSeconds) * time.Second,
		SkipSuccessfulRequests: false,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		Next: func(c *fiber.Ctx) bool {
			path := c.Path()
			// Keep webhook, static assets, and websocket handshake out of global limiter.
			return path == webhookPath ||
				strings.HasPrefix(path, strings.TrimRight(basePath, "/")+"/statics") ||
				strings.HasPrefix(path, strings.TrimRight(basePath, "/")+"/assets") ||
				strings.HasPrefix(path, strings.TrimRight(basePath, "/")+"/components") ||
				strings.HasPrefix(path, strings.TrimRight(basePath, "/")+"/ws")
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"code":    "RATE_LIMITED",
				"message": "too many requests",
			})
		},
	})
}
