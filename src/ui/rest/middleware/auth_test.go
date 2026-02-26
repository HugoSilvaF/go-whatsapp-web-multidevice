package middleware

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func newAuthTestApp(accounts map[string]string, token string) *fiber.App {
	app := fiber.New()
	app.Use(RequireAuth(accounts, token, nil))
	app.Get("/ok", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestRequireAuth_NoAuthConfigured(t *testing.T) {
	app := newAuthTestApp(nil, "")
	req := httptest.NewRequest("GET", "/ok", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequireAuth_BasicAuthSuccess(t *testing.T) {
	app := newAuthTestApp(map[string]string{"admin": "secret"}, "")
	cred := base64.StdEncoding.EncodeToString([]byte("admin:secret"))

	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set("Authorization", "Basic "+cred)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequireAuth_BasicAuthInvalid(t *testing.T) {
	app := newAuthTestApp(map[string]string{"admin": "secret"}, "")
	cred := base64.StdEncoding.EncodeToString([]byte("admin:wrong"))

	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set("Authorization", "Basic "+cred)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAuth_BearerTokenSuccess(t *testing.T) {
	app := newAuthTestApp(nil, "token-123")
	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequireAuth_APIKeyHeaderSuccess(t *testing.T) {
	app := newAuthTestApp(nil, "token-123")
	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set(APIKeyHeader, "token-123")
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequireScope_ForAPIKey(t *testing.T) {
	app := fiber.New()
	app.Use(RequireAuth(nil, "", func(_ context.Context, rawKey string) (*APIKeyValidationResult, error) {
		if rawKey == "gowa.test.secret" {
			return &APIKeyValidationResult{KeyID: "test", Scopes: []string{"messages:send"}}, nil
		}
		return nil, assert.AnError
	}))
	app.Post("/send", RequireScope("messages:send"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	app.Post("/devices", RequireScope("devices:manage"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	reqOK := httptest.NewRequest("POST", "/send", nil)
	reqOK.Header.Set(APIKeyHeader, "gowa.test.secret")
	respOK, err := app.Test(reqOK)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, respOK.StatusCode)

	reqForbidden := httptest.NewRequest("POST", "/devices", nil)
	reqForbidden.Header.Set(APIKeyHeader, "gowa.test.secret")
	respForbidden, err := app.Test(reqForbidden)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, respForbidden.StatusCode)
}
