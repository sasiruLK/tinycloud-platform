package middleware

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestAuthMiddleware_PublicEndpoints(t *testing.T) {
	app := fiber.New()
	app.Use(AuthMiddleware())
	app.Get("/v1/health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	tests := []struct {
		name string
		path string
	}{
		{"health endpoint", "/v1/health"},
		{"metrics endpoint", "/metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			resp, err := app.Test(req)
			assert.NoError(t, err)
			assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		})
	}
}

func TestAuthMiddleware_ProtectedMissingHeader(t *testing.T) {
	app := fiber.New()
	app.Use(AuthMiddleware())
	app.Get("/v1/apps", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/v1/apps", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Unauthorized: missing X-Auth-Request-User header")
}

func TestAuthMiddleware_ProtectedWithValidHeader(t *testing.T) {
	app := fiber.New()
	app.Use(AuthMiddleware())
	app.Get("/v1/apps", func(c *fiber.Ctx) error {
		user := c.Locals("user")
		return c.JSON(fiber.Map{"user": user})
	})

	req := httptest.NewRequest("GET", "/v1/apps", nil)
	req.Header.Set("X-Auth-Request-User", "sasiruLK")
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "sasiruLK")
}
