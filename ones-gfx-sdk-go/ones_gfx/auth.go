package ones_gfx

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// AuthProvider is the interface for authentication strategies.
// The transport layer calls Apply() to attach credentials and Refresh()
// when it receives a 401.
type AuthProvider interface {
	Apply(headers map[string]string)
	Refresh() error
	NeedsProactiveRefresh() bool
	Close() error
}

// JWTAuth implements bearer-token authentication with automatic refresh.
// Both the access token and refresh token are rotated on every refresh
// (per the ONES API contract).
type JWTAuth struct {
	accessToken  string
	refreshToken string
	refreshURL   string
	accessExp    int64 // Unix timestamp (seconds) of access token expiry
	config       authConfig
	mu           sync.Mutex // Protects concurrent refresh attempts
	httpClient   *http.Client
}

// NewJWTAuth constructs a JWTAuth instance with the given credentials.
// The access token must be a JWT so the SDK can read its exp claim for
// proactive refresh. If it's not a JWT, proactive refresh is disabled
// and only reactive (401-driven) refresh will occur.
func NewJWTAuth(accessToken, refreshToken, refreshURL string, opts ...AuthOption) (*JWTAuth, error) {
	if accessToken == "" || refreshToken == "" {
		return nil, fmt.Errorf("accessToken and refreshToken are required")
	}
	if refreshURL == "" {
		return nil, fmt.Errorf("refreshURL is required")
	}
	cfg := defaultAuthConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	// Build HTTP client for refresh calls
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.verifyTLS,
		},
	}
	httpClient := &http.Client{
		Timeout:   cfg.refreshTimeout,
		Transport: transport,
	}

	auth := &JWTAuth{
		accessToken:  accessToken,
		refreshToken: refreshToken,
		refreshURL:   refreshURL,
		config:       cfg,
		httpClient:   httpClient,
	}

	// Extract exp claim from access token for proactive refresh
	auth.accessExp = extractJWTExp(accessToken)

	return auth, nil
}

// Apply attaches the current access token as a Bearer header.
func (a *JWTAuth) Apply(headers map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	headers["Authorization"] = "Bearer " + a.accessToken
}

// NeedsProactiveRefresh returns true if the access token is within the
// buffer window of expiry. If the token isn't a JWT (accessExp == 0),
// this always returns false.
func (a *JWTAuth) NeedsProactiveRefresh() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.accessExp == 0 {
		return false
	}
	now := time.Now().Unix()
	bufferSeconds := int64(a.config.proactiveRefreshBuffer.Seconds())
	return now >= (a.accessExp - bufferSeconds)
}

// Refresh exchanges the refresh token for a new access+refresh token pair.
// Thread-safe: if multiple goroutines call this concurrently, only one
// actually hits the network; others wait and see the updated token.
func (a *JWTAuth) Refresh() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Double-checked locking: another goroutine may have refreshed while
	// we were waiting for the lock.
	if a.accessExp > 0 {
		now := time.Now().Unix()
		bufferSeconds := int64(a.config.proactiveRefreshBuffer.Seconds())
		if now < (a.accessExp - bufferSeconds) {
			// Token is still fresh
			return nil
		}
	}

	return a.doRefresh()
}

// doRefresh performs the actual refresh HTTP call. Caller must hold a.mu.
func (a *JWTAuth) doRefresh() error {
	reqBody, err := json.Marshal(map[string]string{
		"refresh_token": a.refreshToken,
	})
	if err != nil {
		return &TransportError{Message: "failed to marshal refresh request", Cause: err}
	}

	req, err := http.NewRequest("POST", a.refreshURL, bytes.NewReader(reqBody))
	if err != nil {
		return &TransportError{Message: "failed to build refresh request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return &TransportError{Message: "failed to reach refresh endpoint", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		errMsg := "token refresh failed"
		if msg, ok := body["error"].(string); ok {
			errMsg = msg
		} else if msg, ok := body["message"].(string); ok {
			errMsg = msg
		}
		return &AuthenticationError{
			APIError: APIError{
				Message:      errMsg,
				StatusCode:   resp.StatusCode,
				ResponseBody: body,
			},
		}
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return &AuthenticationError{
			APIError: APIError{
				Message:      "refresh response missing tokens",
				StatusCode:   resp.StatusCode,
				ResponseBody: nil,
			},
		}
	}

	if payload.AccessToken == "" || payload.RefreshToken == "" {
		return &AuthenticationError{
			APIError: APIError{
				Message:      "refresh response missing access_token or refresh_token",
				StatusCode:   resp.StatusCode,
				ResponseBody: payload,
			},
		}
	}

	// Update tokens
	a.accessToken = payload.AccessToken
	a.refreshToken = payload.RefreshToken
	a.accessExp = extractJWTExp(payload.AccessToken)

	// If exp claim wasn't readable, fall back to server-supplied expires_in
	if a.accessExp == 0 && payload.ExpiresIn > 0 {
		a.accessExp = time.Now().Unix() + int64(payload.ExpiresIn)
	}

	// Invoke callback if registered
	if a.config.onTokenRefresh != nil {
		a.config.onTokenRefresh(payload.AccessToken, payload.RefreshToken, payload.ExpiresIn)
	}

	return nil
}

// Close is a no-op for JWTAuth (no persistent connections to clean up).
func (a *JWTAuth) Close() error {
	return nil
}

// extractJWTExp decodes a JWT's payload (no signature verification) and
// returns the exp claim as Unix timestamp (seconds). Returns 0 if the token
// isn't a valid JWT or has no exp claim.
func extractJWTExp(token string) int64 {
	parts := splitJWT(token)
	if len(parts) != 3 {
		return 0
	}

	// Decode payload (base64url)
	payloadSegment := parts[1]
	// Add padding if needed
	switch len(payloadSegment) % 4 {
	case 2:
		payloadSegment += "=="
	case 3:
		payloadSegment += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payloadSegment)
	if err != nil {
		return 0
	}

	var claims struct {
		Exp interface{} `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return 0
	}

	// exp may be float64 or int
	switch v := claims.Exp.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

// splitJWT splits a JWT into [header, payload, signature].
func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	if start < len(token) {
		parts = append(parts, token[start:])
	}
	return parts
}
