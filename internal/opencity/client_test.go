package opencity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchBranding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lang/api/tenants/info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name": "Comune di Test",
			"meta": [
				"{\"favicon\":\"http://example.com/fav.png\",\"logo\":\"http://example.com/logo.png\"}"
			]
		}`))
	}))
	defer server.Close()

	b, err := FetchBranding(server.URL)
	if err != nil {
		t.Fatalf("FetchBranding: %v", err)
	}

	if b.Nome != "Comune di Test" {
		t.Errorf("expected Nome = 'Comune di Test', got %q", b.Nome)
	}
	if b.Favicon != "http://example.com/fav.png" {
		t.Errorf("expected Favicon = 'http://example.com/fav.png', got %q", b.Favicon)
	}
	if b.Logo != "http://example.com/logo.png" {
		t.Errorf("expected Logo = 'http://example.com/logo.png', got %q", b.Logo)
	}
}

func TestFetchBrandingError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FetchBranding(server.URL)
	if err == nil {
		t.Error("expected error, got nil")
	}
}
