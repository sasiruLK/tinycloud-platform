package api

import (
	"encoding/json"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// OpenAPISpec builds and serves the OpenAPI 3.0 spec for TinyCloud API
func OpenAPISpec(c *fiber.Ctx) error {
	spec := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "TinyCloud API",
			"description": "API for managing GitOps-driven applications on Kubernetes via Argo CD",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name":  "TinyCloud",
				"url":   "https://github.com/sasiruLK/tinycloud-platform",
				"email": "support@tinycloud.local",
			},
		},
		"servers": []map[string]interface{}{
			{"url": "https://api.tinycloud.local/v1", "description": "Production"},
			{"url": "http://localhost:8080/v1", "description": "Local development"},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Health check",
					"description": "Returns API health status and version",
					"tags":        []string{"system"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "API is healthy",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/HealthResponse"},
								},
							},
						},
					},
				},
			},
			"/apps": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List applications",
					"description": "Returns a paginated list of all Argo CD applications",
					"tags":        []string{"apps"},
					"parameters": []map[string]interface{}{
						{
							"name": "limit", "in": "query", "schema": map[string]interface{}{"type": "integer", "default": 20},
							"description": "Maximum number of items to return (1-100)",
						},
						{
							"name": "offset", "in": "query", "schema": map[string]interface{}{"type": "integer", "default": 0},
							"description": "Number of items to skip",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of applications",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/PaginatedAppsResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"500": errorResponse(),
					},
				},
				"post": map[string]interface{}{
					"summary":     "Create application",
					"description": "Queues a source repository build. The build coordinator commits GitOps manifests after the runner pushes an image.",
					"tags":        []string{"apps"},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/CreateAppRequest"},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Build queued",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/CreateAppResponse"},
								},
							},
						},
						"400": errorResponse(),
						"401": unauthorizedResponse(),
						"409": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/apps/{name}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get application details",
					"description": "Returns detailed information about a single application",
					"tags":        []string{"apps"},
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Application name"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Application details",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/AppDetailResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"404": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/apps/{name}/logs": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get application logs",
					"description": "Returns pod logs for an application",
					"tags":        []string{"apps"},
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Application name"},
						{"name": "container", "in": "query", "schema": map[string]interface{}{"type": "string", "default": "app"}, "description": "Container name"},
						{"name": "tail", "in": "query", "schema": map[string]interface{}{"type": "integer", "default": 100}, "description": "Number of log lines to return"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Pod logs",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/LogResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"404": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/apps/{name}/sync": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Sync application",
					"description": "Triggers an Argo CD sync for the specified application",
					"tags":        []string{"apps"},
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Application name"},
					},
					"responses": map[string]interface{}{
						"202": map[string]interface{}{
							"description": "Sync triggered",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/SyncResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"404": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/apps/{name}/suspend": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Suspend application",
					"description": "Scales app to zero replicas via GitOps commit. History is preserved.",
					"tags":        []string{"apps"},
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Application name"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "App suspended",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/SuspendResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"404": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/apps/{name}/rollback": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Rollback application",
					"description": "Rolls back an application to a specific git revision",
					"tags":        []string{"apps"},
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Application name"},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/RollbackRequest"},
							},
						},
					},
					"responses": map[string]interface{}{
						"202": map[string]interface{}{
							"description": "Rollback initiated",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/RollbackResponse"},
								},
							},
						},
						"400": errorResponse(),
						"401": unauthorizedResponse(),
						"404": errorResponse(),
						"409": errorResponse(),
						"422": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/apps/{name}/restore": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Restore application",
					"description": "Restores an application from rollback state back to main branch",
					"tags":        []string{"apps"},
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Application name"},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/RestoreRequest"},
							},
						},
					},
					"responses": map[string]interface{}{
						"202": map[string]interface{}{
							"description": "Restore initiated",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/RestoreResponse"},
								},
							},
						},
						"400": errorResponse(),
						"401": unauthorizedResponse(),
						"404": errorResponse(),
						"409": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/github/repos": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List GitHub repositories",
					"description": "Returns the authenticated user's GitHub repositories for the repo picker",
					"tags":        []string{"github"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of repositories",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/GitHubReposResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"503": errorResponse(),
						"502": errorResponse(),
					},
				},
			},
			"/builds/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get build status",
					"description": "Returns the status of a queued or running build",
					"tags":        []string{"builds"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Build ID"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Build details",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/BuildJob"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"404": errorResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/builds/{id}/logs": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get build logs",
					"description": "Returns real-time build logs",
					"tags":        []string{"builds"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Build ID"},
						{"name": "after", "in": "query", "schema": map[string]interface{}{"type": "integer", "default": 0}, "description": "Only return logs after this sequence number"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Build logs",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/BuildLogsResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"500": errorResponse(),
					},
				},
			},
			"/rollbacks": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List rollback history",
					"description": "Returns the rollback/restore history for all applications",
					"tags":        []string{"rollbacks"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Rollback history",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/RollbacksResponse"},
								},
							},
						},
						"401": unauthorizedResponse(),
						"500": errorResponse(),
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"ErrorResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"error":     map[string]interface{}{"type": "string", "example": "not_found"},
						"message":   map[string]interface{}{"type": "string", "example": "App not found"},
						"requestId": map[string]interface{}{"type": "string", "example": "550e8400-e29b-41d4-a716-446655440000"},
						"status":    map[string]interface{}{"type": "integer", "example": 404},
					},
					"required": []string{"error", "message", "requestId", "status"},
				},
				"HealthResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"status":  map[string]interface{}{"type": "string", "example": "healthy"},
								"version": map[string]interface{}{"type": "string", "example": "1.0.0"},
								"gitops":  map[string]interface{}{"type": "string", "example": "self-managed-v4"},
								"build":   map[string]interface{}{"type": "string", "example": "standalone-coordinator-runner"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"App": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":           map[string]interface{}{"type": "string"},
						"namespace":      map[string]interface{}{"type": "string"},
						"health":         map[string]interface{}{"type": "string", "example": "Healthy"},
						"syncStatus":     map[string]interface{}{"type": "string", "example": "Synced"},
						"revision":       map[string]interface{}{"type": "string"},
						"imageTag":       map[string]interface{}{"type": "string"},
						"targetRevision": map[string]interface{}{"type": "string"},
						"rollbackStatus": map[string]interface{}{"type": "string", "example": "normal"},
					},
				},
				"Resource": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"kind":   map[string]interface{}{"type": "string"},
						"name":   map[string]interface{}{"type": "string"},
						"status": map[string]interface{}{"type": "string"},
					},
				},
				"AppDetailResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"allOf": []map[string]interface{}{
								{"$ref": "#/components/schemas/App"},
							},
							"properties": map[string]interface{}{
								"repo":      map[string]interface{}{"type": "string"},
								"path":      map[string]interface{}{"type": "string"},
								"resources": map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/components/schemas/Resource"}},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"PaginatedAppsResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"apps": map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/components/schemas/App"}},
							},
						},
						"pagination": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"limit":  map[string]interface{}{"type": "integer"},
								"offset": map[string]interface{}{"type": "integer"},
								"total":  map[string]interface{}{"type": "integer"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"LogResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"pod":       map[string]interface{}{"type": "string"},
								"container": map[string]interface{}{"type": "string"},
								"lines":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"SyncResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"operationId": map[string]interface{}{"type": "string"},
								"status":      map[string]interface{}{"type": "string"},
								"message":     map[string]interface{}{"type": "string"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"CreateAppRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":     map[string]interface{}{"type": "string", "example": "my-app"},
						"repoUrl":  map[string]interface{}{"type": "string", "example": "https://github.com/user/my-app"},
						"ref":      map[string]interface{}{"type": "string", "example": "main"},
						"replicas": map[string]interface{}{"type": "integer", "example": 1},
						"port":     map[string]interface{}{"type": "integer", "example": 8080, "default": 8080, "enum": []int{8080}},
						"env":      map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}},
					},
					"required": []string{"name", "repoUrl"},
				},
				"CreateAppResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"appName": map[string]interface{}{"type": "string"},
								"buildId": map[string]interface{}{"type": "string"},
								"status":  map[string]interface{}{"type": "string", "example": "queued"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"SuspendResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"name":    map[string]interface{}{"type": "string"},
								"status":  map[string]interface{}{"type": "string", "example": "suspended"},
								"message": map[string]interface{}{"type": "string"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"RollbackRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"targetRevision": map[string]interface{}{"type": "string", "description": "40-character git SHA"},
						"reason":         map[string]interface{}{"type": "string"},
						"initiatedBy":    map[string]interface{}{"type": "string"},
					},
					"required": []string{"targetRevision", "reason"},
				},
				"RollbackResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"rollbackId":     map[string]interface{}{"type": "string"},
								"app":            map[string]interface{}{"type": "string"},
								"rollbackBranch": map[string]interface{}{"type": "string"},
								"targetRevision": map[string]interface{}{"type": "string"},
								"status":         map[string]interface{}{"type": "string"},
								"createdAt":      map[string]interface{}{"type": "string", "format": "date-time"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"RestoreRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"reason":      map[string]interface{}{"type": "string"},
						"initiatedBy": map[string]interface{}{"type": "string"},
					},
					"required": []string{"reason"},
				},
				"RestoreResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"restoreId":          map[string]interface{}{"type": "string"},
								"app":                map[string]interface{}{"type": "string"},
								"restoredToRevision": map[string]interface{}{"type": "string"},
								"status":             map[string]interface{}{"type": "string"},
								"createdAt":          map[string]interface{}{"type": "string", "format": "date-time"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"GitHubReposResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"repos": map[string]interface{}{
									"type":  "array",
									"items": map[string]interface{}{"$ref": "#/components/schemas/GitHubRepo"},
								},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"GitHubRepo": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":          map[string]interface{}{"type": "string"},
						"fullName":      map[string]interface{}{"type": "string"},
						"url":           map[string]interface{}{"type": "string"},
						"defaultBranch": map[string]interface{}{"type": "string"},
						"language":      map[string]interface{}{"type": "string"},
						"private":       map[string]interface{}{"type": "boolean"},
					},
				},
				"BuildJob": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id":                map[string]interface{}{"type": "string"},
								"appName":           map[string]interface{}{"type": "string"},
								"repoUrl":           map[string]interface{}{"type": "string"},
								"ref":               map[string]interface{}{"type": "string"},
								"commitSha":         map[string]interface{}{"type": "string"},
								"framework":         map[string]interface{}{"type": "string"},
								"image":             map[string]interface{}{"type": "string"},
								"tag":               map[string]interface{}{"type": "string"},
								"status":            map[string]interface{}{"type": "string", "enum": []string{"queued", "running", "succeeded", "failed"}},
								"attempts":          map[string]interface{}{"type": "integer"},
								"replicas":          map[string]interface{}{"type": "integer"},
								"port":              map[string]interface{}{"type": "integer"},
								"gitopsCommitSha":   map[string]interface{}{"type": "string"},
								"gitopsPath":        map[string]interface{}{"type": "string", "example": "apps/my-app"},
								"deployStatus":      map[string]interface{}{"type": "string", "example": "deployed"},
								"argoSyncStatus":    map[string]interface{}{"type": "string", "example": "Synced"},
								"argoHealth":        map[string]interface{}{"type": "string", "example": "Healthy"},
								"appUrl":            map[string]interface{}{"type": "string", "example": "https://my-app.sasiru.lk/"},
								"verificationError": map[string]interface{}{"type": "string"},
								"error":             map[string]interface{}{"type": "string"},
								"createdAt":         map[string]interface{}{"type": "string", "format": "date-time"},
								"updatedAt":         map[string]interface{}{"type": "string", "format": "date-time"},
								"startedAt":         map[string]interface{}{"type": "string", "format": "date-time"},
								"finishedAt":        map[string]interface{}{"type": "string", "format": "date-time"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"BuildLogsResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"lines": map[string]interface{}{
									"type":  "array",
									"items": map[string]interface{}{"$ref": "#/components/schemas/BuildLogLine"},
								},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
				"BuildLogLine": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"sequence":  map[string]interface{}{"type": "integer"},
						"timestamp": map[string]interface{}{"type": "string", "format": "date-time"},
						"stream":    map[string]interface{}{"type": "string"},
						"message":   map[string]interface{}{"type": "string"},
					},
				},
				"RollbacksResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"data": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"version":     map[string]interface{}{"type": "string"},
								"generatedAt": map[string]interface{}{"type": "string"},
								"apps":        map[string]interface{}{"type": "object"},
							},
						},
						"requestId": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}

	c.Set("Content-Type", "application/json")
	return c.Status(http.StatusOK).SendString(mustJSON(spec))
}

func unauthorizedResponse() map[string]interface{} {
	return map[string]interface{}{
		"description": "Unauthorized",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{"$ref": "#/components/schemas/ErrorResponse"},
			},
		},
	}
}

func errorResponse() map[string]interface{} {
	return map[string]interface{}{
		"description": "Error",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{"$ref": "#/components/schemas/ErrorResponse"},
			},
		},
	}
}

func mustJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
