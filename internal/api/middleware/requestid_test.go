package middleware

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
)

func TestRequestID_GeneratesNewID(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"rid": c.Locals("requestId")})
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))
}

func TestRequestID_PropagatesExistingID(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"rid": c.Locals("requestId")})
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "existing-id-123")
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, "existing-id-123", resp.Header.Get("X-Request-ID"))
}
