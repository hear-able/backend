// Package ai wraps the OpenAI HTTP API for transcription and structured
// evaluation. Business packages depend on small interfaces they define
// themselves, so this package can be swapped for another provider.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrRateLimited means the provider returned 429 even after a retry.
	ErrRateLimited = errors.New("ai provider rate limited")
	// ErrUnavailable covers timeouts, network failures and 5xx responses.
	ErrUnavailable = errors.New("ai provider unavailable")
	// ErrBadResponse means the provider answered but the output was unusable.
	ErrBadResponse = errors.New("ai provider returned an invalid response")
)

type Config struct {
	APIKey          string
	BaseURL         string
	EvalModel       string
	TranscribeModel string
	Timeout         time.Duration
	RetryBackoff    time.Duration
}

type Client struct {
	cfg    Config
	http   *http.Client
	logger *slog.Logger
}

func NewClient(cfg Config, logger *slog.Logger) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = time.Second
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: &http.Client{}, logger: logger}
}

func (c *Client) EvalModel() string { return c.cfg.EvalModel }

// Transcribe sends audio to the transcription endpoint and returns the text.
// The audio is never logged or persisted.
func (c *Client) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	build := func() (*http.Request, error) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(audio); err != nil {
			return nil, err
		}
		_ = mw.WriteField("model", c.cfg.TranscribeModel)
		_ = mw.WriteField("language", "id")
		_ = mw.WriteField("response_format", "json")
		if err := mw.Close(); err != nil {
			return nil, err
		}
		req, err := http.NewRequest(http.MethodPost, c.cfg.BaseURL+"/audio/transcriptions", &body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return req, nil
	}

	respBody, err := c.do(ctx, "transcribe", c.cfg.TranscribeModel, build)
	if err != nil {
		return "", err
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("%w: decode transcription: %v", ErrBadResponse, err)
	}
	return strings.TrimSpace(out.Text), nil
}

// completeJSON runs a chat completion constrained to the given JSON schema
// and unmarshals the model's JSON output into dst.
func (c *Client) completeJSON(ctx context.Context, op, systemPrompt string, userPayload any, schemaName string, schema map[string]any, dst any) error {
	userJSON, err := json.Marshal(userPayload)
	if err != nil {
		return fmt.Errorf("marshal ai payload: %w", err)
	}
	reqBody, err := json.Marshal(map[string]any{
		"model": c.cfg.EvalModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": string(userJSON)},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   schemaName,
				"strict": true,
				"schema": schema,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal ai request: %w", err)
	}

	build := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	respBody, err := c.do(ctx, op, c.cfg.EvalModel, build)
	if err != nil {
		return err
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return fmt.Errorf("%w: decode completion: %v", ErrBadResponse, err)
	}
	if len(out.Choices) == 0 {
		return fmt.Errorf("%w: no choices", ErrBadResponse)
	}
	msg := out.Choices[0].Message
	if msg.Refusal != nil && *msg.Refusal != "" {
		return fmt.Errorf("%w: model refused", ErrBadResponse)
	}
	if msg.Content == nil || out.Choices[0].FinishReason == "length" {
		return fmt.Errorf("%w: empty or truncated content", ErrBadResponse)
	}
	if err := json.Unmarshal([]byte(*msg.Content), dst); err != nil {
		return fmt.Errorf("%w: decode model json: %v", ErrBadResponse, err)
	}
	return nil
}

// do sends the request with a per-attempt timeout and retries exactly once on
// 429, 5xx or a transport error.
func (c *Client) do(ctx context.Context, op, model string, build func() (*http.Request, error)) ([]byte, error) {
	const maxAttempts = 2
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %v", ErrUnavailable, ctx.Err())
			case <-time.After(c.cfg.RetryBackoff):
			}
		}

		body, retry, err := c.attempt(ctx, build)
		c.logger.InfoContext(ctx, "ai request",
			"op", op, "model", model, "attempt", attempt, "ok", err == nil)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) attempt(ctx context.Context, build func() (*http.Request, error)) (body []byte, retry bool, err error) {
	attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	req, err := build()
	if err != nil {
		return nil, false, fmt.Errorf("build ai request: %w", err)
	}
	req = req.WithContext(attemptCtx)
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, true, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}

	c.logger.DebugContext(ctx, "ai response", "status", resp.StatusCode, "latency_ms", time.Since(start).Milliseconds())

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, true, ErrRateLimited
	case resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode >= 400:
		// 4xx other than 429 is a request/config problem; retrying won't help.
		return nil, false, fmt.Errorf("%w: status %d", ErrBadResponse, resp.StatusCode)
	}
	return body, false, nil
}
