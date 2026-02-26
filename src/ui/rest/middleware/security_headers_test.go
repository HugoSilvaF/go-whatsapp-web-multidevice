package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders_AddsHeaders(t *testing.T) {
	app := fiber.New()
	app.Use(SecurityHeaders())
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "SAMEORIGIN", resp.Header.Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
}
