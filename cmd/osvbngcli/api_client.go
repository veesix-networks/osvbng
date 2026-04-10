package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

type APIClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

type cachedContract struct {
	ETag string          `json:"etag"`
	JSON json.RawMessage `json:"json"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}

func newAPIClient(server string) (*APIClient, error) {
	baseURL, err := normalizeServerURL(server)
	if err != nil {
		return nil, err
	}

	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (c *APIClient) baseURLString() string {
	return strings.TrimRight(c.baseURL.String(), "/")
}

func (c *APIClient) loadContract(ctx context.Context) (*Contract, error) {
	cached, _ := loadCachedContract(c.baseURLString())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolvePath("/api/openapi.json"), nil)
	if err != nil {
		return nil, fmt.Errorf("build OpenAPI request: %w", err)
	}
	if cached != nil && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch OpenAPI spec: %w", err)
	}
	defer resp.Body.Close()

	var specJSON []byte
	var etag string

	switch resp.StatusCode {
	case http.StatusOK:
		specJSON, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read OpenAPI spec: %w", err)
		}
		etag = resp.Header.Get("ETag")
		if err := saveCachedContract(c.baseURLString(), &cachedContract{ETag: etag, JSON: specJSON}); err != nil {
			return nil, fmt.Errorf("save cached OpenAPI spec: %w", err)
		}
	case http.StatusNotModified:
		if cached == nil || len(cached.JSON) == 0 {
			return nil, fmt.Errorf("server returned 304 for /api/openapi.json but no cached contract exists")
		}
		specJSON = cached.JSON
	default:
		return nil, decodeAPIError(resp)
	}

	var spec openapi3.T
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, fmt.Errorf("parse OpenAPI spec: %w", err)
	}

	_ = etag
	return buildContract(&spec)
}

func (c *APIClient) doJSON(ctx context.Context, method, path string, query url.Values, body interface{}, out interface{}) error {
	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		requestBody = bytes.NewReader(payload)
	}

	requestURL := c.resolvePath(path)
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, requestBody)
	if err != nil {
		return fmt.Errorf("build %s %s request: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return decodeAPIError(resp)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s response: %w", method, path, err)
	}

	return nil
}

func (c *APIClient) resolvePath(path string) string {
	base := strings.TrimRight(c.baseURL.String(), "/")
	return base + path
}

func decodeAPIError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("request failed with status %d", resp.StatusCode)}
	}

	var apiErr errorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error) != "" {
		return &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(apiErr.Error)}
	}
	if len(body) == 0 {
		return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("request failed with status %d", resp.StatusCode)}
	}

	return &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
}

func normalizeServerURL(server string) (*url.URL, error) {
	trimmed := strings.TrimSpace(server)
	if trimmed == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid server URL %q", server)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func loadCachedContract(baseURL string) (*cachedContract, error) {
	path, err := contractCachePath(baseURL)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cached cachedContract
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}
	return &cached, nil
}

func saveCachedContract(baseURL string, cached *cachedContract) error {
	path, err := contractCachePath(baseURL)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func contractCachePath(baseURL string) (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(baseURL))
	return filepath.Join(cacheRoot, "osvbngcli", "contracts", hex.EncodeToString(sum[:])+".json"), nil
}
