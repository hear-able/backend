package ai

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(Config{
		APIKey: "test-key", BaseURL: srv.URL, EvalModel: "eval-model", TranscribeModel: "stt-model",
		Timeout: 500 * time.Millisecond, RetryBackoff: time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return c, &calls
}

func completion(content string) string {
	b := strings.ReplaceAll(content, `"`, `\"`)
	return `{"choices":[{"message":{"content":"` + b + `"},"finish_reason":"stop"}]}`
}

const validListening = `{"score":80,"criteria":{"main_idea":90,"details":70,"relevance":85},"explanation":"Bagus","strengths":["ide utama"],"missed_points":[" "],"suggestions":[]}`

func TestEvaluateListeningSuccess(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"strict":true`) {
			t.Error("expected strict json schema")
		}
		io.WriteString(w, completion(validListening))
	})
	got, err := c.EvaluateListening(t.Context(), ListeningInput{UserAnswer: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Score != 80 || got.Criteria.MainIdea != 90 || got.Model != "eval-model" {
		t.Fatalf("unexpected result %+v", got)
	}
	if len(got.MissedPoints) != 0 {
		t.Fatalf("blank list items should be dropped, got %q", got.MissedPoints)
	}
}

func TestEvaluateListeningRejectsMalformedAndOutOfRange(t *testing.T) {
	for name, content := range map[string]string{
		"not json":     `hello`,
		"out of range": strings.Replace(validListening, `"score":80`, `"score":250`, 1),
		"no explain":   strings.Replace(validListening, `"Bagus"`, `""`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, completion(content))
			})
			_, err := c.EvaluateListening(t.Context(), ListeningInput{})
			if !errors.Is(err, ErrBadResponse) {
				t.Fatalf("want ErrBadResponse, got %v", err)
			}
		})
	}
}

func TestRetriesOnceOn429(t *testing.T) {
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.EvaluateListening(t.Context(), ListeningInput{})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("want exactly 2 attempts, got %d", calls.Load())
	}
}

func TestRetrySucceedsAfter5xx(t *testing.T) {
	var n atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, completion(validListening))
	})
	if _, err := c.EvaluateListening(t.Context(), ListeningInput{}); err != nil {
		t.Fatal(err)
	}
}

func TestNoRetryOn400(t *testing.T) {
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	_, err := c.EvaluateListening(t.Context(), ListeningInput{})
	if !errors.Is(err, ErrBadResponse) || calls.Load() != 1 {
		t.Fatalf("want 1 call and ErrBadResponse, got %d calls, err %v", calls.Load(), err)
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	_, err := c.EvaluateListening(t.Context(), ListeningInput{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestTranscribe(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("model") != "stt-model" || r.FormValue("language") != "id" {
			t.Errorf("bad form %v", r.MultipartForm.Value)
		}
		io.WriteString(w, `{"text":"  Selamat pagi  "}`)
	})
	got, err := c.Transcribe(t.Context(), []byte("audio"), "recording.webm")
	if err != nil || got != "Selamat pagi" {
		t.Fatalf("got %q, %v", got, err)
	}
}
