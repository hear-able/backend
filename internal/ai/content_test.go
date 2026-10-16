package ai

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSuggestListeningPrompt(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "listening_instruction") || !strings.Contains(string(body), "Selamat pagi") {
			t.Errorf("unexpected request body %s", body)
		}
		io.WriteString(w, completion(`{"prompt":"  Dengarkan audio berikut.  "}`))
	})
	got, err := c.SuggestListeningPrompt(t.Context(), "Selamat pagi")
	if err != nil || got != "Dengarkan audio berikut." {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestSuggestListeningPromptRejectsEmpty(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, completion(`{"prompt":" "}`))
	})
	if _, err := c.SuggestListeningPrompt(t.Context(), "x"); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

func TestAssessDifficulty(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"enum":[1,2,3,4]`) {
			t.Errorf("expected difficulty enum in schema, got %s", body)
		}
		io.WriteString(w, completion(`{"difficulty":3,"explanation":"Banyak detail angka."}`))
	})
	got, err := c.AssessDifficulty(t.Context(), DifficultyInput{ModuleType: "listening", Items: []DifficultyItem{{Transcription: "x"}}})
	if err != nil || got.Difficulty != 3 || got.Explanation != "Banyak detail angka." {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestAssessDifficultyRejectsOutOfRange(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, completion(`{"difficulty":7,"explanation":"x"}`))
	})
	if _, err := c.AssessDifficulty(t.Context(), DifficultyInput{}); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}
