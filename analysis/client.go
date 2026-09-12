package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 30 * time.Second
	// Rich-document JSON may contain up to eight million Unicode code points
	// plus JSON escaping. Keep one protocol-level byte bound that can carry the
	// accepted document response without making it unbounded.
	defaultMaxResponseSize = 64 << 20
)

// ClientConfig constrains one authenticated loopback connection. A caller may
// inject an HTTP client for tests, but the endpoint itself must remain local.
type ClientConfig struct {
	BaseURL          string
	BearerToken      string
	RequestTimeout   time.Duration
	MaxResponseBytes int64
	HTTPClient       *http.Client
}

// Client is the capability API offered by one sidecar session. Consumer
// modules wrap it behind their own sentence, noun-phrase, or conversion ports.
type Client struct {
	baseURL          *url.URL
	bearerToken      string
	requestTimeout   time.Duration
	maxResponseBytes int64
	httpClient       *http.Client
	cache            Cache
}

// NewClient creates a client that cannot be redirected or proxied away from
// the selected loopback address.
func NewClient(config ClientConfig) (*Client, error) {
	baseURL, err := validateLoopbackBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(config.BearerToken)
	if token == "" {
		return nil, errors.New("analysis bearer token is required")
	}
	requestTimeout := config.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	maxResponseBytes := config.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseSize
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		target := baseURL.Host
		dialer := &net.Dialer{Timeout: min(requestTimeout, 5*time.Second)}
		httpClient = &http.Client{
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, network, target)
				},
				ResponseHeaderTimeout: requestTimeout,
			},
		}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("analysis redirects are disabled")
	}
	return &Client{
		baseURL: baseURL, bearerToken: token, requestTimeout: requestTimeout,
		maxResponseBytes: maxResponseBytes, httpClient: &clientCopy,
	}, nil
}

// Health authenticates the process, validates protocol identity, and checks
// that all capabilities required by the consuming index mode are installed.
func (c *Client) Health(ctx context.Context, required ...Capability) (Health, error) {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return Health{}, unavailable("health", false, errors.New("analysis client is not configured"))
	}
	var health Health
	payload, err := c.doJSON(ctx, http.MethodGet, "/v1/health", nil)
	if err != nil {
		return Health{}, err
	}
	if err := decodeSingleJSON(payload, &health); err != nil {
		return Health{}, unavailable("response", false, err)
	}
	if err := health.validate(required); err != nil {
		return Health{}, unavailable("health", false, err)
	}
	return health, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, requestBody []byte) ([]byte, error) {
	var body io.Reader
	if requestBody != nil {
		body = bytes.NewReader(requestBody)
	}
	contentLength := int64(-1)
	if requestBody != nil {
		contentLength = int64(len(requestBody))
	}
	return c.do(ctx, method, path, body, "application/json", contentLength)
}

func (c *Client) do(
	ctx context.Context,
	method string,
	path string,
	body io.Reader,
	contentType string,
	contentLength int64,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, unavailable("request", true, err)
	}
	requestContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequestWithContext(requestContext, method, endpoint.String(), body)
	if err != nil {
		return nil, unavailable("request", false, err)
	}
	request.Header.Set("Authorization", "Bearer "+c.bearerToken)
	request.Header.Set("Accept", "application/json")
	if body != nil && strings.TrimSpace(contentType) != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if body != nil && contentLength >= 0 {
		request.ContentLength = contentLength
	}
	httpResponse, err := c.httpClient.Do(request)
	if err != nil {
		return nil, unavailable("request", true, err)
	}
	defer httpResponse.Body.Close()
	payload, err := readBounded(httpResponse.Body, c.maxResponseBytes)
	if err != nil {
		return nil, unavailable("response", false, err)
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		var envelope errorEnvelope
		if err := decodeSingleJSON(payload, &envelope); err != nil || strings.TrimSpace(envelope.Error.Code) == "" {
			return nil, failed("remote", httpResponse.StatusCode >= http.StatusInternalServerError, errors.New("analysis returned an invalid error response"))
		}
		return nil, failed("remote_"+sanitizeCode(envelope.Error.Code), envelope.Error.Retryable, errors.New("analysis returned a controlled error"))
	}
	return payload, nil
}

func validateLoopbackBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse analysis base URL: %w", err)
	}
	if parsed.Scheme != "http" {
		return nil, errors.New("analysis base URL must use http")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("analysis base URL must not contain credentials, path, query, or fragment")
	}
	host := net.ParseIP(parsed.Hostname())
	if host == nil || !host.IsLoopback() {
		return nil, errors.New("analysis base URL must use a loopback IP address")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("analysis base URL must contain a valid port")
	}
	parsed.Path = "/"
	return parsed, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read analysis response: %w", err)
	}
	if int64(len(payload)) > limit {
		return nil, errors.New("analysis response exceeds configured limit")
	}
	return payload, nil
}

func decodeSingleJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode analysis response: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("analysis response contains trailing JSON")
	}
	return nil
}

func sanitizeCode(code string) string {
	var builder strings.Builder
	for _, value := range strings.ToLower(strings.TrimSpace(code)) {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_' {
			builder.WriteRune(value)
		}
	}
	if builder.Len() == 0 {
		return "failed"
	}
	return builder.String()
}
