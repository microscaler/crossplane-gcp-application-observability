// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package gcprest is a small JSON client for Google REST APIs, used by the
// controllers whose resources have no Terraform equivalent.
package gcprest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Client calls Google REST APIs with an authenticated HTTP client.
type Client struct {
	http *http.Client
}

// New returns a Client.
func New(h *http.Client) *Client { return &Client{http: h} }

// APIError is a non-2xx response.
type APIError struct {
	Code    int
	Status  string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.Code, e.Status, e.Message)
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool { return code(err) == http.StatusNotFound }

// IsConflict reports whether err is a 409 (for example ALREADY_EXISTS).
func IsConflict(err error) bool { return code(err) == http.StatusConflict }

func code(err error) int {
	if e, ok := err.(*APIError); ok { //nolint:errorlint // only ever returned unwrapped
		return e.Code
	}
	return 0
}

// Do sends one request. in, when not nil, is sent as JSON; out, when not nil,
// receives the JSON response.
func (c *Client) Do(ctx context.Context, method, rawURL string, q url.Values, in, out any) error {
	if len(q) > 0 {
		rawURL += "?" + q.Encode()
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		return &APIError{Code: resp.StatusCode, Status: e.Error.Status, Message: e.Error.Message}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// List pages through a collection, calling fn for each item of field.
func (c *Client) List(ctx context.Context, rawURL string, q url.Values, field string, fn func(json.RawMessage) error) error {
	query := url.Values{}
	for k, v := range q {
		query[k] = v
	}
	for {
		var page map[string]json.RawMessage
		if err := c.Do(ctx, http.MethodGet, rawURL, query, nil, &page); err != nil {
			return err
		}
		var items []json.RawMessage
		if raw, ok := page[field]; ok {
			if err := json.Unmarshal(raw, &items); err != nil {
				return err
			}
		}
		for _, it := range items {
			if err := fn(it); err != nil {
				return err
			}
		}
		var token string
		if raw, ok := page["nextPageToken"]; ok {
			_ = json.Unmarshal(raw, &token)
		}
		if token == "" {
			return nil
		}
		query.Set("pageToken", token)
	}
}
