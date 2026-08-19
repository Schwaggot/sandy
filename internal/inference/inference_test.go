package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const llamaCppBody = `{"data":[
  {"id":"Qwen3.8-27B-think-MTP","object":"model","meta":{"n_ctx":262144}},
  {"id":"Gemma-4-26B-A4B-it","object":"model","meta":{"n_ctx":131072}}
]}`

func TestListParsesModelsAndContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path: %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth header: %q", got)
		}
		w.Write([]byte(llamaCppBody))
	}))
	defer srv.Close()

	models, err := List(context.Background(), "openai", srv.URL+"/v1", "k", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models: %v", models)
	}
	if models[0].ID != "Qwen3.8-27B-think-MTP" || models[0].Context != 262144 {
		t.Errorf("first model: %+v", models[0])
	}
}

func TestListAnthropicUsesAPIKeyHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("headers: %v", r.Header)
		}
		w.Write([]byte(`{"data":[{"id":"claude-x"}]}`))
	}))
	defer srv.Close()

	models, err := List(context.Background(), "anthropic", srv.URL+"/v1", "k", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "claude-x" {
		t.Errorf("models: %v", models)
	}
}

func TestListTrailingSlashAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path: %q", r.URL.Path)
		}
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	// An endpoint serving nothing is an error, not an empty success: sandy
	// must fall back to the agent's own model rather than pin an empty id.
	if _, err := List(context.Background(), "openai", srv.URL+"/v1/", "", ""); err == nil {
		t.Fatal("expected error for empty model list")
	}
}

func TestListStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := List(context.Background(), "openai", srv.URL+"/v1", "", "")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err: %v", err)
	}
}

// A hostname that only resolves inside the container must fall back to the
// add_host IP for the host-side lookup.
func TestListFallsBackToAddHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"local"}]}`))
	}))
	defer srv.Close()

	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	models, err := List(context.Background(), "openai", "http://container-only-name:"+port+"/v1", "", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "local" {
		t.Errorf("models: %v", models)
	}
}

func TestSelect(t *testing.T) {
	models := []Model{{ID: "Gemma-4-26B"}, {ID: "Qwen3.8-27B-think-MTP"}}

	cases := []struct {
		name   string
		prefer []string
		want   string
	}{
		{"no preference takes the first served", nil, "Gemma-4-26B"},
		{"glob, case-insensitive", []string{"*THINK*"}, "Qwen3.8-27B-think-MTP"},
		{"exact id", []string{"Qwen3.8-27B-think-MTP"}, "Qwen3.8-27B-think-MTP"},
		{"first matching pattern wins", []string{"*nothing*", "gemma*"}, "Gemma-4-26B"},
		{"no match falls back to the first served", []string{"*absent*"}, "Gemma-4-26B"},
		{"blank patterns are skipped", []string{"", "*think*"}, "Qwen3.8-27B-think-MTP"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Select(models, tc.prefer)
			if !ok || got.ID != tc.want {
				t.Errorf("got %q (%v), want %q", got.ID, ok, tc.want)
			}
		})
	}

	if _, ok := Select(nil, nil); ok {
		t.Error("empty listing must not select")
	}
}
