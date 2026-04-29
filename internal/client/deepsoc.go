package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"traffic-go/internal/domain"
)

type DeepSOCClient struct {
	BaseURL  string
	APIKey   string
	Username string
	Password string
	HTTP     *http.Client
}

func (c DeepSOCClient) Enabled() bool {
	return strings.TrimSpace(c.BaseURL) != ""
}

func (c DeepSOCClient) CreateEvent(ctx context.Context, payload domain.Event, idempotencyKey string) (map[string]any, error) {
	if !c.Enabled() {
		return nil, errors.New("deepsoc base url is empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/event/create", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return out, errors.New("deepsoc create event failed")
	}
	return out, nil
}
