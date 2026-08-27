package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/response"
	"github.com/sasiruLK/tinycloud-platform/internal/infra"
)

// GetInfra returns the infrastructure snapshot: nodes, alarms, ingress,
// backups, uptime and the free-tier budget, assembled from this Instance's
// Providers.
//
// It is always served from the cache, never from a live fan-out, so a poll from
// the dashboard costs nothing and a Provider is called at most once a minute. A
// snapshot with some Capabilities missing is still returned — the gaps are null
// and named in `warnings`.
func (h *Handler) GetInfra(c *fiber.Ctx) error {
	if h.Infra == nil {
		return response.JSONError(c, fiber.StatusServiceUnavailable, "infra_unavailable",
			"Infrastructure reporting is not configured")
	}

	snap, err := h.Infra.Get()
	if err != nil {
		// Nothing has been collected yet: no Provider answered the first
		// refresh, or the Substrate credentials behind one could not be
		// resolved. Say so instead of returning an empty dashboard.
		if errors.Is(err, infra.ErrNotReady) {
			return response.JSONError(c, fiber.StatusServiceUnavailable, "infra_unavailable",
				"Infrastructure snapshot unavailable: "+err.Error())
		}
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to read infrastructure snapshot")
	}

	return response.JSON(c, snap)
}
