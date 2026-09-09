package inference

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		_, _ = w.Write([]byte(llamaCppBody))
	}))
	defer srv.Close()

	models, err := List(context.Background(), Target{Protocol: "openai", BaseURL: srv.URL + "/v1", APIKey: "k"})
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
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-x"}]}`))
	}))
	defer srv.Close()

	models, err := List(context.Background(), Target{Protocol: "anthropic", BaseURL: srv.URL + "/v1", APIKey: "k"})
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
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	// An endpoint serving nothing is an error, not an empty success: sandy
	// must fall back to the agent's own model rather than pin an empty id.
	if _, err := List(context.Background(), Target{Protocol: "openai", BaseURL: srv.URL + "/v1/"}); err == nil {
		t.Fatal("expected error for empty model list")
	}
}

func TestListStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := List(context.Background(), Target{Protocol: "openai", BaseURL: srv.URL + "/v1"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err: %v", err)
	}
}

// A hostname that only resolves inside the container must fall back to the
// add_host IP for the host-side lookup.
func TestListFallsBackToAddHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"local"}]}`))
	}))
	defer srv.Close()

	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	models, err := List(context.Background(), Target{Protocol: "openai", BaseURL: "http://container-only-name:" + port + "/v1", AddHost: "127.0.0.1"})
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

func TestListTLSNeedsCACert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(llamaCppBody))
	}))
	defer srv.Close()

	target := Target{Protocol: "openai", BaseURL: srv.URL + "/v1"}
	if _, err := List(context.Background(), target); err == nil {
		t.Fatal("an untrusted server certificate must not be accepted")
	}

	ca := filepath.Join(t.TempDir(), "ca.crt")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	target.CACert = ca
	models, err := List(context.Background(), target)
	if err != nil {
		t.Fatalf("ca_cert should make the endpoint reachable: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models: %v", models)
	}
}

func TestListCACertErrors(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "not-a-cert")
	if err := os.WriteFile(junk, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"missing": filepath.Join(dir, "absent.crt"),
		"non-PEM": junk,
	} {
		_, err := List(context.Background(), Target{
			Protocol: "openai", BaseURL: "https://example.invalid/v1", CACert: path,
		})
		if err == nil || !strings.Contains(err.Error(), "ca_cert") {
			t.Errorf("%s: want a ca_cert error, got %v", name, err)
		}
	}
}

// writeTLSCert returns a certificate valid for dnsName only (no IP SAN) plus
// the PEM file to trust it.
func writeTLSCert(t *testing.T, dnsName string) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: dnsName},
		DNSNames:              []string{dnsName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(path, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	return pair, path
}

// The add_host retry must only redirect the connection, not the identity:
// the certificate is valid for the configured hostname and carries no IP SAN,
// so rewriting the URL to the IP would fail verification.
func TestListAddHostRetryKeepsTLSHostname(t *testing.T) {
	const name = "gpu.internal"
	cert, ca := writeTLSCert(t, name)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Host, name+":") {
			t.Errorf("Host header should stay the configured name, got %q", r.Host)
		}
		_, _ = w.Write([]byte(llamaCppBody))
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	models, err := List(context.Background(), Target{
		Protocol: "openai",
		BaseURL:  "https://" + name + ":" + u.Port() + "/v1",
		AddHost:  "127.0.0.1",
		CACert:   ca,
	})
	if err != nil {
		t.Fatalf("pinned retry should verify against %s: %v", name, err)
	}
	if len(models) != 2 {
		t.Fatalf("models: %v", models)
	}
}
