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
)

type LLMClient struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

type LLMHealth struct {
	Configured bool   `json:"configured"`
	OK         bool   `json:"ok"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (c LLMClient) Enabled() bool {
	return strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.APIKey) != ""
}

func (c LLMClient) HealthCheck(ctx context.Context) LLMHealth {
	h := LLMHealth{
		Configured: c.Enabled(),
		BaseURL:    strings.TrimRight(strings.TrimSpace(c.BaseURL), "/"),
		Model:      strings.TrimSpace(c.Model),
	}
	if h.BaseURL != "" {
		h.Endpoint = h.BaseURL + "/chat/completions"
	}
	if h.Model == "" {
		h.Model = "deepseek-chat"
	}
	if !h.Configured {
		h.Error = "LLM_BASE_URL or LLM_API_KEY is empty"
		return h
	}

	payload := map[string]any{
		"model": h.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "Return only OK."},
			{"role": "user", "content": "health"},
		},
		"stream":      false,
		"max_tokens":  4,
		"temperature": 0,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Endpoint, bytes.NewReader(b))
	if err != nil {
		h.Error = err.Error()
		return h
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	start := time.Now()
	resp, err := httpClient.Do(req)
	h.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		h.Error = err.Error()
		return h
	}
	defer resp.Body.Close()

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		h.Error = err.Error()
		return h
	}
	if resp.StatusCode >= 400 {
		h.Error = fmt.Sprintf("llm request failed: status=%d error=%v", resp.StatusCode, out.Error)
		return h
	}
	if len(out.Choices) == 0 {
		h.Error = "llm returned empty choices"
		return h
	}
	h.OK = true
	return h
}

func (c LLMClient) Chat(ctx context.Context, prompt string) (string, error) {
	if !c.Enabled() {
		return "LLM 未配置。已收到消息：" + prompt, nil
	}
	payload := map[string]any{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个安全运营中心 SOC 助手，回答要可执行、简洁。"},
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", errors.New("llm request failed")
	}
	if len(out.Choices) == 0 {
		return "", errors.New("llm returned empty choices")
	}
	return out.Choices[0].Message.Content, nil
}
