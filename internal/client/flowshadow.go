package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type FlowShadowClient struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func (c FlowShadowClient) Enabled() bool {
	return strings.TrimSpace(c.BaseURL) != ""
}

func (c FlowShadowClient) ListEvents(ctx context.Context, since string, limit int) ([]map[string]any, error) {
	if !c.Enabled() {
		return nil, errors.New("flowshadow base url is empty")
	}
	start := parseSinceToEpoch(since)
	form := url.Values{}
	form.Set("req_type", "aggre")
	form.Set("starttime", strconv.FormatInt(start, 10))
	form.Set("endtime", strconv.FormatInt(time.Now().UTC().Unix(), 10))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/d/event", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if c.APIKey != "" && c.APIKey != "change-me" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	items := unwrapList(data)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (c FlowShadowClient) GetFlow(ctx context.Context, flowID string) (map[string]any, error) {
	return c.getJSON(ctx, "/d/feature?id="+url.QueryEscape(flowID))
}

func (c FlowShadowClient) GetRelatedFlows(ctx context.Context, flowID, window, relType string, limit int) (map[string]any, error) {
	q := url.Values{}
	q.Set("flow_id", flowID)
	q.Set("window", window)
	q.Set("rel_type", relType)
	q.Set("limit", strconv.Itoa(limit))
	return c.getJSON(ctx, "/d/feature?"+q.Encode())
}

func (c FlowShadowClient) GetAsset(ctx context.Context, ip string) (map[string]any, error) {
	return c.getJSON(ctx, "/d/mo?op=get&moip="+url.QueryEscape(ip))
}

func (c FlowShadowClient) PreparePCAP(ctx context.Context, flowID string) (map[string]any, error) {
	return map[string]any{"pcap_id": "flow-" + flowID, "status": "prepared"}, nil
}

func (c FlowShadowClient) GetPCAP(ctx context.Context, pcapID string) (map[string]any, error) {
	if !c.Enabled() {
		return nil, errors.New("flowshadow base url is empty")
	}
	return map[string]any{"pcap_id": pcapID, "status": "not_downloaded", "message": "请在 repository 层按 devid/time_sec/time_usec/ip/port 转换到 /d/evidence?download=true"}, nil
}

func (c FlowShadowClient) Proxy(w http.ResponseWriter, r *http.Request) {
	if !c.Enabled() {
		http.Error(w, `{"status":"error","message":"FLOWSHADOW_BASE_URL is empty"}`, http.StatusBadGateway)
		return
	}
	target := strings.TrimRight(c.BaseURL, "/") + r.URL.RequestURI()
	var body io.Reader
	if r.Body != nil {
		body = r.Body
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header = r.Header.Clone()
	if c.APIKey != "" && c.APIKey != "change-me" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (c FlowShadowClient) getJSON(ctx context.Context, path string) (map[string]any, error) {
	if !c.Enabled() {
		return nil, errors.New("flowshadow base url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if c.APIKey != "" && c.APIKey != "change-me" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
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
		return out, errors.New("flowshadow request failed")
	}
	return out, nil
}

func parseSinceToEpoch(since string) int64 {
	s := strings.TrimSpace(since)
	if s == "" {
		return time.Now().UTC().Add(-10 * time.Minute).Unix()
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t.UTC().Unix()
	}
	return time.Now().UTC().Add(-10 * time.Minute).Unix()
}

func unwrapList(data any) []map[string]any {
	out := []map[string]any{}
	switch v := data.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
	case map[string]any:
		for _, key := range []string{"items", "data", "rows", "list", "result", "records"} {
			if arr, ok := v[key].([]any); ok {
				for _, item := range arr {
					if m, ok := item.(map[string]any); ok {
						out = append(out, m)
					}
				}
				return out
			}
		}
		out = append(out, v)
	}
	return out
}
