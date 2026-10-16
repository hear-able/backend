// Package storage uploads files to Supabase Storage over its REST API.
package storage

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
)

type Config struct {
	URL        string // project URL, e.g. https://xyz.supabase.co
	ServiceKey string
	Bucket     string
	Timeout    time.Duration
}

type Supabase struct {
	cfg  Config
	http *http.Client
}

func NewSupabase(cfg Config) *Supabase {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	return &Supabase{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Upload stores data at path in the bucket and returns its public URL. The
// bucket must be public for the URL to be playable by the mobile app.
func (s *Supabase) Upload(ctx context.Context, path string, data []byte, contentType string) (string, error) {
	endpoint := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.cfg.URL, s.cfg.Bucket, escapePath(path))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build upload request: %w", err)
	}
	s.authorize(req)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "false")

	if err := s.send(req); err != nil {
		return "", fmt.Errorf("upload %s: %w", path, err)
	}
	return s.PublicURL(path), nil
}

// Delete removes objects from the bucket. Used to clean up uploads when the
// database insert that should reference them fails.
func (s *Supabase) Delete(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	body, _ := json.Marshal(map[string][]string{"prefixes": paths})
	endpoint := fmt.Sprintf("%s/storage/v1/object/%s", s.cfg.URL, s.cfg.Bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build delete request: %w", err)
	}
	s.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	if err := s.send(req); err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}
	return nil
}

func (s *Supabase) PublicURL(path string) string {
	return fmt.Sprintf("%s/storage/v1/object/public/%s/%s", s.cfg.URL, s.cfg.Bucket, escapePath(path))
}

func (s *Supabase) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+s.cfg.ServiceKey)
	req.Header.Set("apikey", s.cfg.ServiceKey)
}

func (s *Supabase) send(req *http.Request) error {
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}
