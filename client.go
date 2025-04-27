package espoclient

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Constants for HTTP Methods
const (
	MethodGet     = http.MethodGet
	MethodPost    = http.MethodPost
	MethodPut     = http.MethodPut
	MethodDelete  = http.MethodDelete
	MethodOptions = http.MethodOptions
)

const defaultApiPath = "/api/v1/"

// Header represents a single HTTP header.
type Header struct {
	Name  string
	Value string
}

// Client manages communication with the EspoCRM API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	apiPath    string
	username   *string
	password   *string
	apiKey     *string
	secretKey  *string
}

// Response holds the API response details.
type Response struct {
	StatusCode  int
	ContentType string
	Headers     http.Header
	Body        []byte // Raw response body
}

// EspoError is a general error from the client.
type EspoError struct {
	Message string
	Cause   error
}

func (e *EspoError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("espoclient: %s: %v", e.Message, e.Cause)
	}
	return "espoclient: " + e.Message
}

// ResponseError is returned when the API responds with a non-2xx status code.
type ResponseError struct {
	Response     *Response
	ErrorMessage string // Content of X-Status-Reason header if available
}

func (e *ResponseError) Error() string {
	if e.ErrorMessage != "" {
		return fmt.Sprintf("espoclient: API error (HTTP %d): %s", e.Response.StatusCode, e.ErrorMessage)
	}
	return fmt.Sprintf("espoclient: API error (HTTP %d)", e.Response.StatusCode)
}

// GetParsedBody attempts to unmarshal the JSON response body into the provided value.
func (r *Response) GetParsedBody(v any) error {
	if len(r.Body) == 0 {
		return fmt.Errorf("response body is empty")
	}
	// Check if content type indicates JSON, although we try anyway
	if !strings.Contains(strings.ToLower(r.ContentType), "application/json") {
		// Optionally return an error here if strict JSON type is required
		// return fmt.Errorf("response content type is not JSON (%s)", r.ContentType)
	}
	err := json.Unmarshal(r.Body, v)
	if err != nil {
		return fmt.Errorf("failed to parse JSON body: %w", err)
	}
	return nil
}

// GetBodyString returns the raw response body as a string.
func (r *Response) GetBodyString() string {
	return string(r.Body)
}

// NewClient creates a new EspoCRM API client.
// urlStr should be the base URL of your EspoCRM instance (e.g., "https://myespo.example.com").
// port is optional; if nil, the default for the scheme (80/443) is used.
func NewClient(urlStr string, port *int) (*Client, error) {
	if !strings.HasSuffix(urlStr, "/") {
		urlStr += "/"
	}
	baseURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, &EspoError{Message: "invalid base URL", Cause: err}
	}

	if port != nil {
		baseURL.Host = baseURL.Hostname() + ":" + strconv.Itoa(*port)
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Second * 30, // Default timeout
		},
		apiPath: defaultApiPath,
	}, nil
}

// SetHTTPClient allows setting a custom http.Client (e.g., for custom transport, timeouts).
func (c *Client) SetHTTPClient(client *http.Client) *Client {
	c.httpClient = client
	return c
}

// SetUsernameAndPassword sets credentials for Basic Authentication. Not recommended.
func (c *Client) SetUsernameAndPassword(username, password string) *Client {
	c.username = &username
	c.password = &password
	c.apiKey = nil    // Clear other auth methods
	c.secretKey = nil // Clear other auth methods
	return c
}

// SetApiKey sets the API Key for authentication.
func (c *Client) SetApiKey(apiKey string) *Client {
	c.apiKey = &apiKey
	c.username = nil // Clear other auth methods
	c.password = nil // Clear other auth methods
	// Keep secretKey if it was set for potential HMAC auth
	return c
}

// SetSecretKey sets the Secret Key for HMAC authentication (requires API Key to also be set).
func (c *Client) SetSecretKey(secretKey string) *Client {
	c.secretKey = &secretKey
	c.username = nil // Clear other auth methods
	c.password = nil // Clear other auth methods
	return c
}

// Request sends a request to the EspoCRM API.
// method: HTTP method (e.g., espoclient.MethodGet).
// path: The API endpoint path (e.g., "Lead", "Account/some-id").
// data: The request payload.
//   - For GET: map[string]string or url.Values for query parameters.
//   - For POST/PUT/DELETE:
//   - Any struct or map[string]any will be JSON-encoded.
//   - url.Values will be form-urlencoded.
//   - io.Reader will be streamed directly (Content-Type header should be set manually).
//   - []byte will be sent directly (Content-Type header should be set manually).
//   - string will be sent directly (Content-Type header should be set manually).
//
// headers: A map of additional headers to send.
func (c *Client) Request(method, path string, data any, headers map[string]string) (*Response, error) {
	// 1. Compose URL
	rel, err := url.Parse(strings.TrimPrefix(c.apiPath, "/") + strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, &EspoError{Message: "invalid API path", Cause: err}
	}
	fullURL := c.baseURL.ResolveReference(rel)

	// 2. Prepare Request Body and Query Params
	var reqBody io.Reader
	contentType := "" // Detected or default content type

	if method == MethodGet && data != nil {
		query := fullURL.Query()
		switch v := data.(type) {
		case map[string]string:
			for key, val := range v {
				query.Set(key, val)
			}
		case url.Values:
			for key, vals := range v {
				for _, val := range vals { // Add potentially multiple values
					query.Add(key, val)
				}
			}
		default:
			return nil, &EspoError{Message: fmt.Sprintf("unsupported data type for GET query parameters: %T", data)}
		}
		fullURL.RawQuery = query.Encode()
	} else if data != nil {
		// Handle non-GET request body
		switch v := data.(type) {
		case io.Reader:
			reqBody = v // Stream directly
		case []byte:
			reqBody = bytes.NewReader(v)
		case string:
			reqBody = strings.NewReader(v)
		case url.Values:
			reqBody = strings.NewReader(v.Encode())
			contentType = "application/x-www-form-urlencoded"
		default:
			// Assume JSON for structs, maps, etc.
			jsonData, err := json.Marshal(data)
			if err != nil {
				return nil, &EspoError{Message: "failed to marshal data to JSON", Cause: err}
			}
			reqBody = bytes.NewReader(jsonData)
			contentType = "application/json"
		}
	}

	// 3. Create Request
	req, err := http.NewRequest(method, fullURL.String(), reqBody)
	if err != nil {
		return nil, &EspoError{Message: "failed to create HTTP request", Cause: err}
	}

	// 4. Set Headers (including authentication and content type)

	// Authentication Headers (HMAC takes precedence)
	if c.apiKey != nil && c.secretKey != nil {
		// HMAC Auth
		hmacString := method + " /" + strings.TrimPrefix(path, "/")
		mac := hmac.New(sha256.New, []byte(*c.secretKey))
		mac.Write([]byte(hmacString))
		signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		authPart := base64.StdEncoding.EncodeToString([]byte(*c.apiKey + ":" + signature))
		req.Header.Set("X-Hmac-Authorization", authPart)
	} else if c.apiKey != nil {
		// API Key Auth
		req.Header.Set("X-Api-Key", *c.apiKey)
	} else if c.username != nil && c.password != nil {
		// Basic Auth
		req.SetBasicAuth(*c.username, *c.password)
	}

	// Content-Type Header (if detected/defaulted and not overridden by user)
	userContentTypeSet := false
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			userContentTypeSet = true
			// User explicitly set Content-Type, respect it
			// Note: net/http canonicalizes header keys (e.g., "content-type" -> "Content-Type")
			req.Header.Set(k, v)
		} else {
			req.Header.Set(k, v)
		}
	}

	if contentType != "" && !userContentTypeSet {
		req.Header.Set("Content-Type", contentType)
	}

	// 5. Execute Request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &EspoError{Message: "HTTP request execution failed", Cause: err}
	}
	defer resp.Body.Close() // Ensure body is always closed

	// 6. Read Response Body
	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &EspoError{Message: "failed to read response body", Cause: err}
	}

	// 7. Create Response Object
	apiResponse := &Response{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     resp.Header,
		Body:        respBodyBytes,
	}

	// 8. Check for API Errors (non-2xx status)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Use ResponseError to wrap the Response object
		responseErr := &ResponseError{
			Response:     apiResponse,
			ErrorMessage: resp.Header.Get("X-Status-Reason"), // Get potential error message
		}
		return nil, responseErr
	}

	// 9. Return Success Response
	return apiResponse, nil
}
