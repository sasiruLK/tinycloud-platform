package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// AuthMiddleware checks for X-Auth-User header from OAuth2 Proxy.
// It allows unauthenticated access to /v1/health and /metrics.
func AuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()

		// Skip authentication for health and metrics endpoints
		if path == "/v1/health" || path == "/metrics" {
			return c.Next()
		}

		// Check X-Auth-User header set by OAuth2 Proxy
		user := c.Get("X-Auth-User")
		if user == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   true,
				"message": "Unauthorized: missing X-Auth-User header. Please authenticate via OAuth2.",
			})
		}

		// Store authenticated user in context for handlers
		c.Locals("user", user)
		return c.Next()
	}
}
