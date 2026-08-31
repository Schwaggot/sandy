// Package inference discovers which models an endpoint currently serves.
//
// Sandy never stores a model id: the served model is read from the endpoint's
// /models listing at launch time, so a server that swaps models does not
// require a config edit.
package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// List fetches the models served at baseURL. protocol selects the auth header
// style ("openai" or "anthropic"); apiKey may be empty for unauthenticated
// local servers. addHost, when set, is the IP to retry against if the URL's
// hostname does not resolve from the host (it only resolves in the container).
func List(ctx context.Context, protocol, baseURL, apiKey, addHost string) ([]Model, error) {
	models, err := list(ctx, protocol, baseURL, apiKey)
	if err == nil || strings.TrimSpace(addHost) == "" {
		return models, err
	}
	fallback, ferr := swapHost(baseURL, addHost)
	if ferr != nil {
		return nil, err
	}
	return list(ctx, protocol, fallback, apiKey)
}

func list(ctx context.Context, protocol, baseURL, apiKey string) ([]Model, error) {
	u := strings.TrimSuffix(baseURL, "/") + "/models"
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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

// swapHost rewrites the URL's hostname to ip, keeping the port and path.
func swapHost(raw, ip string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("cannot parse hostname from url %q: %v", raw, err)
	}
	host := ip
	if port := u.Port(); port != "" {
		host = ip + ":" + port
	}
	u.Host = host
	return u.String(), nil
}
