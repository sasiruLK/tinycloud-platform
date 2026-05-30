package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/build/types"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) CreateBuild(ctx context.Context, req types.CreateBuildRequest) (*types.CreateBuildResponse, error) {
	var out types.CreateBuildResponse
	if err := c.do(ctx, http.MethodPost, "/v1/builds", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetBuild(ctx context.Context, id string) (*types.BuildJob, error) {
	var out types.BuildJob
	if err := c.do(ctx, http.MethodGet, "/v1/builds/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetLogs(ctx context.Context, id string, after int64) (*types.BuildLogsResponse, error) {
	var out types.BuildLogsResponse
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/v1/builds/%s/logs?after=%d", id, after), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&errBody)
		if errBody.Error == "" {
			errBody.Error = res.Status
		}
		return errors.New(errBody.Error)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}
