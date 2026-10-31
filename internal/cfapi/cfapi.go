// Package cfapi calls Cloudflare's API, with a login's OAuth token: what's decided on is the HTTP status, never its
// words.
package cfapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Base is Cloudflare's API.
var Base = "https://api.cloudflare.com/client/v4"

// Result is a call to Cloudflare's API as it answered: its HTTP status, which is what's decided on (404 not there,
// 409 a conflict, …: the same for every product), and the envelope its OpenAPI gives every product, whose words are
// only ever shown.
type Result struct {
	Status int
	Errors []struct {
		Code             int    `json:"code"`
		Message          string `json:"message"`
		DocumentationURL string `json:"documentation_url"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

// OK reports whether the call succeeded (2xx).
func (a Result) OK() bool { return a.Status/100 == 2 }

// Said is Cloudflare's own words for a call that didn't succeed: its status, messages and codes, and their
// documentation links when it gives them.
func (a Result) Said() string {
	var out []string
	for _, e := range a.Errors {
		s := fmt.Sprintf("%s (code %d)", e.Message, e.Code)
		if e.DocumentationURL != "" {
			s += ": " + e.DocumentationURL
		}
		out = append(out, s)
	}
	return fmt.Sprintf("HTTP %d: %s", a.Status, strings.Join(out, "; "))
}

// Call calls Cloudflare's API at path (/accounts/…) with token: body, if any, as contentType.
func Call(ctx context.Context, token, method, path string, body []byte, contentType string) (Result, error) {
	var a Result
	req, err := http.NewRequestWithContext(ctx, method, Base+path, bytes.NewReader(body))
	if err != nil {
		return a, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return a, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(&a) // a body that isn't the envelope leaves only the status
	a.Status = resp.StatusCode
	return a, nil
}
