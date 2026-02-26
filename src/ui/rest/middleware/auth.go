package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const APIKeyHeader = "X-API-Key"
const authPrincipalLocalsKey = "auth_principal"

type APIKeyValidationResult struct {
	KeyID  string
	Scopes []string
}

type APIKeyValidator func(ctx context.Context, rawKey string) (*APIKeyValidationResult, error)

type AuthPrincipal struct {
	Type       string
	KeyID      string
	Scopes     map[string]struct{}
	FullAccess bool
}

func (p *AuthPrincipal) HasScope(scope string) bool {
	if p == nil {
		return false
	}
	if p.FullAccess {
		return true
	}
	if _, ok := p.Scopes["*"]; ok {
		return true
	}
	_, ok := p.Scopes[strings.ToLower(strings.TrimSpace(scope))]
	return ok
}

func PrincipalFromCtx(c *fiber.Ctx) (*AuthPrincipal, bool) {
	val := c.Locals(authPrincipalLocalsKey)
	if val == nil {
		return nil, false
	}
	p, ok := val.(*AuthPrincipal)
	return p, ok && p != nil
}

func RequireAuth(accounts map[string]string, token string, keyValidator APIKeyValidator) fiber.Handler {
	token = strings.TrimSpace(token)

	return func(c *fiber.Ctx) error {
		if len(accounts) == 0 && token == "" && keyValidator == nil {
			c.Locals(authPrincipalLocalsKey, &AuthPrincipal{
				Type:       "none",
				FullAccess: true,
			})
			return c.Next()
		}

		if isBasicAuthorized(c.Get("Authorization"), accounts) {
			c.Locals(authPrincipalLocalsKey, &AuthPrincipal{
				Type:       "basic",
				FullAccess: true,
			})
			return c.Next()
		}

		if token != "" && isTokenAuthorized(c.Get("Authorization"), c.Get(APIKeyHeader), token) {
			c.Locals(authPrincipalLocalsKey, &AuthPrincipal{
				Type:       "token",
				FullAccess: true,
			})
			return c.Next()
		}

		if keyValidator != nil {
			apiKeyValue := strings.TrimSpace(c.Get(APIKeyHeader))
			if apiKeyValue == "" {
				apiKeyValue = parseBearerToken(c.Get("Authorization"))
			}
			if apiKeyValue != "" {
				res, err := keyValidator(c.UserContext(), apiKeyValue)
				if err == nil && res != nil {
					scopes := make(map[string]struct{}, len(res.Scopes))
					for _, s := range res.Scopes {
						s = strings.ToLower(strings.TrimSpace(s))
						if s != "" {
							scopes[s] = struct{}{}
						}
					}
					c.Locals(authPrincipalLocalsKey, &AuthPrincipal{
						Type:       "api_key",
						KeyID:      res.KeyID,
						Scopes:     scopes,
						FullAccess: false,
					})
					return c.Next()
				}
			}
		}

		c.Set("WWW-Authenticate", `Basic realm="Restricted"`)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "UNAUTHORIZED",
			"message": "invalid or missing authentication credentials",
		})
	}
}

func RequireScope(scopes ...string) fiber.Handler {
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		s := strings.ToLower(strings.TrimSpace(scope))
		if s != "" {
			normalized = append(normalized, s)
		}
	}

	return func(c *fiber.Ctx) error {
		if len(normalized) == 0 {
			return c.Next()
		}

		principal, ok := PrincipalFromCtx(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    "UNAUTHORIZED",
				"message": "missing authenticated principal",
			})
		}
		if principal.FullAccess {
			return c.Next()
		}
		for _, scope := range normalized {
			if principal.HasScope(scope) {
				return c.Next()
			}
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"code":    "FORBIDDEN_SCOPE",
			"message": "api key does not have the required scope",
			"results": fiber.Map{
				"required_scopes": normalized,
			},
		})
	}
}

func isTokenAuthorized(authorization, apiKeyHeader, configuredToken string) bool {
	bearer := parseBearerToken(authorization)
	if secureEqual(bearer, configuredToken) {
		return true
	}
	return secureEqual(strings.TrimSpace(apiKeyHeader), configuredToken)
}

func IsSecureTokenMatch(provided, expected string) bool {
	return secureEqual(strings.TrimSpace(provided), strings.TrimSpace(expected))
}

func isBasicAuthorized(authorization string, accounts map[string]string) bool {
	if len(accounts) == 0 {
		return false
	}

	auth := strings.TrimSpace(authorization)
	if !strings.HasPrefix(strings.ToLower(auth), "basic ") {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(auth[len("Basic "):]))
	if err != nil {
		return false
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return false
	}

	expectedPassword, ok := accounts[parts[0]]
	if !ok {
		return false
	}

	return secureEqual(parts[1], expectedPassword)
}

func parseBearerToken(authorization string) string {
	auth := strings.TrimSpace(authorization)
	if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return ""
	}
	return strings.TrimSpace(auth[len("Bearer "):])
}

func secureEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
