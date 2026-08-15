package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/response"
	"github.com/sasiruLK/tinycloud-platform/internal/oci"
)

// GetInfra returns the OCI infrastructure snapshot: nodes, alarms, ingress,
// backups, uptime and the Always Free budget.
//
// It is always served from the cache, never from a live fan-out, so a poll from
// the dashboard costs nothing and OCI Monitoring sees at most one refresh a
// minute. A snapshot with some sources missing is still returned — the gaps are
// null and named in `warnings`.
func (h *Handler) GetInfra(c *fiber.Ctx) error {
	if h.Infra == nil {
		return response.JSONError(c, fiber.StatusServiceUnavailable, "infra_unavailable",
			"Infrastructure reporting is not configured")
	}

	snap, err := h.Infra.Get()
	if err != nil {
		// The usual cause is instance principal authentication failing —
		// running off an OCI instance, or the node's dynamic group lacking the
		// read policies. Say so instead of returning an empty dashboard.
		if errors.Is(err, oci.ErrNotReady) {
			return response.JSONError(c, fiber.StatusServiceUnavailable, "infra_unavailable",
				"Infrastructure snapshot unavailable: "+err.Error())
		}
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to read infrastructure snapshot")
	}

	return response.JSON(c, snap)
}
