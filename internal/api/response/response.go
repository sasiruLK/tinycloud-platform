package response

import "github.com/gofiber/fiber/v2"

// HTTPError is a custom error type with structured fields
type HTTPError struct {
	Code    int
	ErrCode string
	Message string
}

func (e *HTTPError) Error() string { return e.Message }

// ErrorResponse is the standard error JSON shape
type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Status    int    `json:"status"`
}

// SuccessResponse wraps any successful payload
type SuccessResponse struct {
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"requestId"`
}

// PaginatedResponse wraps a list payload with pagination metadata
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
	RequestID  string      `json:"requestId"`
}

// Pagination holds slice pagination metadata
type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// RequestID reads the request ID from Fiber locals
func RequestID(c *fiber.Ctx) string {
	if rid := c.Locals("requestId"); rid != nil {
		if s, ok := rid.(string); ok {
			return s
		}
	}
	return ""
}

// User reads the authenticated user from Fiber locals
func User(c *fiber.Ctx) string {
	if u := c.Locals("user"); u != nil {
		if s, ok := u.(string); ok {
			return s
		}
	}
	return ""
}

// JSONError sends a structured error response
func JSONError(c *fiber.Ctx, status int, errCode, message string) error {
	return c.Status(status).JSON(ErrorResponse{
		Error:     errCode,
		Message:   message,
		RequestID: RequestID(c),
		Status:    status,
	})
}

// JSON sends a structured success response
func JSON(c *fiber.Ctx, data interface{}) error {
	return c.JSON(SuccessResponse{
		Data:      data,
		RequestID: RequestID(c),
	})
}

// JSONStatus sends a structured success response with a custom HTTP status
func JSONStatus(c *fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(SuccessResponse{
		Data:      data,
		RequestID: RequestID(c),
	})
}

// JSONPaginated sends a structured paginated response
func JSONPaginated(c *fiber.Ctx, data interface{}, limit, offset, total int) error {
	return c.JSON(PaginatedResponse{
		Data: data,
		Pagination: Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
		RequestID: RequestID(c),
	})
}

// PaginateSlice returns a sliced view and clamps limit/offset
func PaginateSlice(limit, offset, total int) (int, int, int) {
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return limit, offset, end
}
