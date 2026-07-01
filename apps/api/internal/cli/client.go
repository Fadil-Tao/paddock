package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
	apiKey string
}

func NewClient(baseURL string) *Client {
	apiKey := os.Getenv("PADDOCK_API_KEY")
	return &Client{
		baseURL: baseURL + "/api",
		http:    &http.Client{Timeout: 30 * time.Second},
		apiKey: apiKey,
	}
}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("request failed (%d)", e.Status)
	}
	return e.Message
}

func do[T any](c *Client, ctx context.Context, method, path string, body any) (T, error) {
	var out T

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return out, err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return out, err
	}

	req.Header.Set("X-API-Key", c.apiKey )

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	var env responseWrapper[json.RawMessage]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil && err != io.EOF {
		return out, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, &apiError{Status: resp.StatusCode, Message: env.Message}
	}

	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *Client) List(ctx context.Context) ([]Sandbox, error) {
	return do[[]Sandbox](c, ctx, http.MethodGet, "/sandboxes", nil)
}

func (c *Client) Get(ctx context.Context, id string) (Sandbox, error) {
	return do[Sandbox](c, ctx, http.MethodGet, "/sandboxes/"+url.PathEscape(id), nil)
}

func (c *Client) Create(ctx context.Context) (Sandbox, error) {
	return do[Sandbox](c, ctx, http.MethodPost, "/sandboxes", nil)
}

func (c *Client) Remove(ctx context.Context, id string) error {
	_, err := do[json.RawMessage](c, ctx, http.MethodDelete, "/sandboxes/"+url.PathEscape(id), nil)
	return err
}

// apiState maps the CLI verbs to the values the API expects.
var apiState = map[string]string{
	"start": "running",
	"stop":  "stopped",
}

// ChangeState takes the verb "start" or "stop" and sends the matching API state
// ("running" / "stopped") to PATCH /sandbox/{id}.
func (c *Client) ChangeState(ctx context.Context, id, verb string) error {
	state, ok := apiState[verb]
	if !ok {
		return fmt.Errorf("invalid state %q (want start|stop)", verb)
	}
	_, err := do[json.RawMessage](c, ctx, http.MethodPatch, "/sandbox/"+url.PathEscape(id),
		map[string]string{"state": state})
	return err
}

func (c *Client) Exec(ctx context.Context, id string, cmd []string) (ExecResult, error) {
	return do[ExecResult](c, ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/execs",
		map[string][]string{"cmd": cmd})
}

func (c *Client) Logs(ctx context.Context, id string, tail int) (LogsResult, error) {
	path := "/sandboxes/" + url.PathEscape(id) + "/logs"
	if tail > 0 {
		path += "?tail=" + strconv.Itoa(tail)
	}
	return do[LogsResult](c, ctx, http.MethodGet, path, nil)
}
