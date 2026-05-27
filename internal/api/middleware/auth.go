package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/response"
)

// AuthMiddleware checks for X-Auth-Request-User header from OAuth2 Proxy.
// It allows unauthenticated access to /v1/health and /metrics.
func AuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()

		// Skip authentication for health, metrics and openapi endpoints
		if path == "/v1/health" || path == "/metrics" || path == "/openapi.json" {
			return c.Next()
		}

		// Check X-Auth-Request-User header set by OAuth2 Proxy
		user := c.Get("X-Auth-Request-User")
		if user == "" {
			return response.JSONError(c, fiber.StatusUnauthorized, "unauthorized",
				"Missing X-Auth-Request-User header. Please authenticate via OAuth2.")
		}

		// Store authenticated user in context for handlers
		c.Locals("user", user)
		return c.Next()
	}
}
