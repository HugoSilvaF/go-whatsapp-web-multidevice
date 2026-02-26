package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestRequestRateLimit_BlocksAfterMax(t *testing.T) {
	app := fiber.New()
	app.Use(RequestRateLimit(1, 60, ""))
	app.Get("/send/message", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req1 := httptest.NewRequest("GET", "/send/message", nil)
	resp1, err := app.Test(req1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp1.StatusCode)

	req2 := httptest.NewRequest("GET", "/send/message", nil)
	resp2, err := app.Test(req2)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusTooManyRequests, resp2.StatusCode)
}

func TestRequestRateLimit_SkipsChatwootWebhook(t *testing.T) {
	app := fiber.New()
	app.Use(RequestRateLimit(1, 60, "/gowa"))
	app.Post("/gowa/chatwoot/webhook", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req1 := httptest.NewRequest("POST", "/gowa/chatwoot/webhook", nil)
	resp1, err := app.Test(req1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp1.StatusCode)

	req2 := httptest.NewRequest("POST", "/gowa/chatwoot/webhook", nil)
	resp2, err := app.Test(req2)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)
}
