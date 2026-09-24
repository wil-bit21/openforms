// Package client is a small Go client for the openforms HTTP API, used by the CLI.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openforms/openforms/internal/definition"
)

// Bundle is the wire shape of /api/v1/definitions (export, validate, apply).
type Bundle struct {
	Forms     []definition.Form     `json:"forms"`
	Workflows []definition.Workflow `json:"workflows"`
}

// ApplyItem mirrors definitions.ApplyItem as JSON (spec §7.3).
type ApplyItem struct {
	Kind    string `json:"kind"`
	Slug    string `json:"slug"`
	Version int    `json:"version"`
	Changed bool   `json:"changed"`
	Created bool   `json:"created"`
}

type ApplyResult struct {
	Items []ApplyItem `json:"items"`
}

type ValidateResult struct {
	Valid bool `json:"valid"`
}

// Error is an API error decoded from the error envelope (spec §7.1).
type Error struct {
	Status  int
	Code    string
	Message string
	Details []definition.Problem
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.Status)
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Validate(ctx context.Context, b Bundle) (ValidateResult, error) {
	var out ValidateResult
	err := c.do(ctx, http.MethodPost, "/api/v1/definitions/validate", normalize(b), &out)
	return out, err
}

func (c *Client) Apply(ctx context.Context, b Bundle, dryRun bool) (ApplyResult, error) {
	q := url.Values{"source": {"cli"}}
	if dryRun {
		q.Set("dryRun", "true")
	}
	var out ApplyResult
	err := c.do(ctx, http.MethodPost, "/api/v1/definitions/apply?"+q.Encode(), normalize(b), &out)
	return out, err
}

func (c *Client) Export(ctx context.Context) (Bundle, error) {
	var out Bundle
	err := c.do(ctx, http.MethodGet, "/api/v1/definitions", nil, &out)
	return normalize(out), err
}

func normalize(b Bundle) Bundle {
	if b.Forms == nil {
		b.Forms = []definition.Form{}
	}
	if b.Workflows == nil {
		b.Workflows = []definition.Workflow{}
	}
	return b
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach openforms server at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read response from %s: %w", c.baseURL, err)
	}
	if resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response from %s%s: %w", c.baseURL, path, err)
	}
	return nil
}

func decodeError(status int, data []byte) error {
	var env struct {
		Error struct {
			Code    string               `json:"code"`
			Message string               `json:"message"`
			Details []definition.Problem `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &env); err == nil && env.Error.Code != "" {
		return &Error{Status: status, Code: env.Error.Code, Message: env.Error.Message, Details: env.Error.Details}
	}
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		msg = http.StatusText(status)
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return &Error{Status: status, Code: "http_error", Message: msg}
}
