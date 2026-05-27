package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RequestID generates or propagates a request ID via X-Request-ID header
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rid := c.Get("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Locals("requestId", rid)
		c.Set("X-Request-ID", rid)
		return c.Next()
	}
}
