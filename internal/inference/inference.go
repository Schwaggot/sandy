// Package inference discovers which models an endpoint currently serves.
//
// Sandy never stores a model id: the served model is read from the endpoint's
// /models listing at launch time, so a server that swaps models does not
// require a config edit.
package inference

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// DefaultTimeout bounds a single /models lookup. Resolution is best-effort:
// a slow or down endpoint must not delay the agent's start for long.
const DefaultTimeout = 3 * time.Second

// Model is one entry from an endpoint's /models listing.
type Model struct {
	ID string
	// Context is the server-reported context window, 0 when not advertised.
	Context int
}

// Selection is the model chosen for one endpoint.
type Selection struct {
	Model
	// BaseURL is the endpoint the model was discovered on, as configured
	// (the container-visible URL, not any host-side fallback used to query it).
	BaseURL string
	// Provider is the agent-side provider id the endpoint is registered under.
	Provider string
	// Protocol is the endpoint's wire protocol ("openai" or "anthropic").
	Protocol string
}

// listResponse covers the OpenAI /models shape. llama.cpp and vLLM both add a
// meta block carrying the loaded context window; absent elsewhere.
type listResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Meta struct {
			NCtx int `json:"n_ctx"`
		} `json:"meta"`
	} `json:"data"`
}

// Target is the endpoint to query.
type Target struct {
	// Protocol selects the auth header style ("openai" or "anthropic").
	Protocol string
	BaseURL  string
	// APIKey may be empty for unauthenticated local servers.
	APIKey string
	// AddHost, when set, is the IP to retry against if the URL's hostname
	// does not resolve from the host (it only resolves in the container).
	AddHost string
	// CACert is a host path to a PEM CA bundle added to the system roots for
	// this lookup. Empty uses the system roots alone.
	CACert string
}

// List fetches the models served at the target's URL.
func List(ctx context.Context, t Target) ([]Model, error) {
	client, err := clientFor(t, "")
	if err != nil {
		return nil, err
	}
	models, err := list(ctx, client, t)
	if err == nil || strings.TrimSpace(t.AddHost) == "" {
		return models, err
	}
	// Retry with the hostname pinned to add_host, the way --add-host does
	// inside the container. Only the TCP target changes: the URL, the Host
	// header and the TLS name stay as configured, so an https endpoint is
	// still verified against its own name and not against a bare IP.
	pinned, perr := clientFor(t, t.AddHost)
	if perr != nil {
		return nil, err
	}
	return list(ctx, pinned, t)
}

// clientFor returns the client for one lookup. A ca_cert endpoint gets its own
// root pool: the system roots plus that bundle. Setting RootCAs also switches
// Go to its own verifier, which is what lets an internal CA whose leaf breaks a
// platform policy (macOS caps TLS validity at 398 days) work.
func clientFor(t Target, pinIP string) (*http.Client, error) {
	caCert := strings.TrimSpace(t.CACert)
	if caCert == "" && pinIP == "" {
		return http.DefaultClient, nil
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if caCert != "" {
		pemBytes, err := os.ReadFile(caCert)
		if err != nil {
			return nil, fmt.Errorf("ca_cert: %w", err)
		}
		// An unreadable system pool narrows trust to this bundle alone, which
		// is still the right answer for the endpoint it belongs to.
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("ca_cert: %s holds no PEM certificate", caCert)
		}
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	if pinIP != "" {
		d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			return d.DialContext(ctx, network, net.JoinHostPort(pinIP, port))
		}
	}
	return &http.Client{Transport: tr}, nil
}

func list(ctx context.Context, client *http.Client, t Target) ([]Model, error) {
	protocol, apiKey := t.Protocol, t.APIKey
	u := strings.TrimSuffix(t.BaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		switch protocol {
		case "anthropic":
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		default:
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}

	var lr listResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	out := make([]Model, 0, len(lr.Data))
	for _, d := range lr.Data {
		if d.ID == "" {
			continue
		}
		out = append(out, Model{ID: d.ID, Context: d.Meta.NCtx})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no models served", u)
	}
	return out, nil
}

// Select picks which of the served models to use. prefer holds glob patterns
// (case-insensitive) tried in order; the first pattern matching any served
// model wins. Without preferences, or with none matching, the first served
// model is used - which is the whole listing on a single-model server.
func Select(models []Model, prefer []string) (Model, bool) {
	if len(models) == 0 {
		return Model{}, false
	}
	for _, pattern := range prefer {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		for _, m := range models {
			if ok, err := path.Match(pattern, strings.ToLower(m.ID)); err == nil && ok {
				return m, true
			}
		}
	}
	return models[0], true
}
