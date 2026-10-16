package storage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadAndDelete(t *testing.T) {
	var gotPath, gotAuth, gotType, gotBody, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotAuth, gotType, gotBody = r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(b)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	s := NewSupabase(Config{URL: srv.URL + "/", ServiceKey: "secret", Bucket: "audio"})

	url, err := s.Upload(t.Context(), "volunteer/a.m4a", []byte("data"), "audio/mp4")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/storage/v1/object/audio/volunteer/a.m4a" || gotAuth != "Bearer secret" || gotType != "audio/mp4" || gotBody != "data" {
		t.Fatalf("unexpected upload request %s %s %s %s %q", gotMethod, gotPath, gotAuth, gotType, gotBody)
	}
	if url != srv.URL+"/storage/v1/object/public/audio/volunteer/a.m4a" {
		t.Fatalf("unexpected public url %s", url)
	}

	if err := s.Delete(t.Context(), []string{"volunteer/a.m4a"}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete || gotBody != `{"prefixes":["volunteer/a.m4a"]}` {
		t.Fatalf("unexpected delete request %s %q", gotMethod, gotBody)
	}
}

func TestUploadErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	s := NewSupabase(Config{URL: srv.URL, ServiceKey: "k", Bucket: "audio"})
	if _, err := s.Upload(t.Context(), "x.mp3", []byte("x"), "audio/mpeg"); err == nil {
		t.Fatal("expected error on 403")
	}
}
