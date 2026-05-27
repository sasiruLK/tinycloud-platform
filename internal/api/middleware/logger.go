package middleware

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// StructuredLogger logs every request with method, path, status, duration, user, requestId
func StructuredLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		status := c.Response().StatusCode()
		method := c.Method()
		path := c.Path()

		user := ""
		if u := c.Locals("user"); u != nil {
			user, _ = u.(string)
		}
		rid := ""
		if r := c.Locals("requestId"); r != nil {
			rid, _ = r.(string)
		}

		log.Printf(`{"time":"%s","method":"%s","path":"%s","status":%d,"duration_ms":%.3f,"user":"%s","requestId":"%s"}`,
			time.Now().UTC().Format(time.RFC3339),
			method,
			path,
			status,
			float64(duration.Microseconds())/1000.0,
			user,
			rid,
		)

		return err
	}
}
