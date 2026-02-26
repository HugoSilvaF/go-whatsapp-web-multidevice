package rest

import (
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/apikey"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type Auth struct {
	Service *apikey.Service
}

type createAPIKeyRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expires_in_days"`
}

func InitRestAuth(app fiber.Router, service *apikey.Service) Auth {
	rest := Auth{Service: service}
	app.Get("/auth/keys", rest.ListKeys)
	app.Post("/auth/keys", rest.CreateKey)
	app.Delete("/auth/keys/:id", rest.RevokeKey)
	app.Post("/auth/keys/:id/rotate", rest.RotateKey)
	return rest
}

func (a *Auth) CreateKey(c *fiber.Ctx) error {
	if a.Service == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ResponseData{Status: 503, Code: "API_KEY_SERVICE_UNAVAILABLE", Message: "API key service is unavailable"})
	}

	var req createAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{Status: 400, Code: "BAD_REQUEST", Message: "Invalid request body"})
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Scopes) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{Status: 400, Code: "INVALID_REQUEST", Message: "name and scopes are required"})
	}

	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		t := time.Now().UTC().AddDate(0, 0, req.ExpiresInDays)
		expiresAt = &t
	}

	meta, key, err := a.Service.CreateKey(c.UserContext(), apikey.CreateParams{
		Name:      req.Name,
		Scopes:    req.Scopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{Status: 400, Code: "INVALID_REQUEST", Message: err.Error()})
	}

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "API key created",
		Results: fiber.Map{
			"id":         meta.ID,
			"name":       meta.Name,
			"scopes":     meta.Scopes,
			"expires_at": meta.ExpiresAt,
			"api_key":    key,
		},
	})
}

func (a *Auth) ListKeys(c *fiber.Ctx) error {
	if a.Service == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ResponseData{Status: 503, Code: "API_KEY_SERVICE_UNAVAILABLE", Message: "API key service is unavailable"})
	}
	keys, err := a.Service.ListKeys(c.UserContext(), c.QueryBool("include_revoked", false))
	if err != nil {
		utils.PanicIfNeeded(err)
	}
	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "API keys listed",
		Results: keys,
	})
}

func (a *Auth) RevokeKey(c *fiber.Ctx) error {
	if a.Service == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ResponseData{Status: 503, Code: "API_KEY_SERVICE_UNAVAILABLE", Message: "API key service is unavailable"})
	}
	if err := a.Service.RevokeKey(c.UserContext(), c.Params("id")); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(utils.ResponseData{Status: 404, Code: "NOT_FOUND", Message: err.Error()})
	}
	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "API key revoked",
		Results: nil,
	})
}

func (a *Auth) RotateKey(c *fiber.Ctx) error {
	if a.Service == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ResponseData{Status: 503, Code: "API_KEY_SERVICE_UNAVAILABLE", Message: "API key service is unavailable"})
	}
	meta, key, err := a.Service.RotateKey(c.UserContext(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{Status: 400, Code: "INVALID_REQUEST", Message: err.Error()})
	}
	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "API key rotated",
		Results: fiber.Map{
			"id":      meta.ID,
			"name":    meta.Name,
			"scopes":  meta.Scopes,
			"api_key": key,
		},
	})
}
