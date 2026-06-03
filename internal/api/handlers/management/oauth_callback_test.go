package management

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
)

func TestPostOAuthCallbackCreatesMissingAuthDir(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authDir := filepath.Join(t.TempDir(), "missing-auth")
	state := "test-antigravity-state"
	RegisterOAuthSession(state, "antigravity")
	defer CompleteOAuthSession(state)

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, nil)
	router := gin.New()
	router.POST("/v0/management/oauth-callback", h.PostOAuthCallback)

	body := `{"provider":"antigravity","redirect_url":"http://localhost:59788/oauth-callback?state=test-antigravity-state&code=test-code"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/management/oauth-callback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, w.Code, w.Body.String())
	}

	callbackPath := filepath.Join(authDir, ".oauth-antigravity-"+state+".oauth")
	data, errRead := os.ReadFile(callbackPath)
	if errRead != nil {
		t.Fatalf("expected callback file to be written: %v", errRead)
	}

	var payload oauthCallbackFilePayload
	if errUnmarshal := json.Unmarshal(data, &payload); errUnmarshal != nil {
		t.Fatalf("failed to decode callback payload: %v", errUnmarshal)
	}
	if payload.State != state || payload.Code != "test-code" || payload.Error != "" {
		t.Fatalf("unexpected callback payload: %+v", payload)
	}
}

func TestWriteOAuthCallbackFileForPendingSessionCreatesMissingAuthDirForCallbackProviders(t *testing.T) {
	providers := []string{"anthropic", "codex", "gemini", "antigravity", "xai"}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			authDir := filepath.Join(t.TempDir(), "missing-auth")
			state := provider + "-state"
			RegisterOAuthSession(state, provider)
			defer CompleteOAuthSession(state)

			path, errWrite := WriteOAuthCallbackFileForPendingSession(authDir, provider, state, "code-"+provider, "")
			if errWrite != nil {
				t.Fatalf("expected callback file write to succeed: %v", errWrite)
			}

			data, errRead := os.ReadFile(path)
			if errRead != nil {
				t.Fatalf("expected callback file to be written: %v", errRead)
			}

			var payload oauthCallbackFilePayload
			if errUnmarshal := json.Unmarshal(data, &payload); errUnmarshal != nil {
				t.Fatalf("failed to decode callback payload: %v", errUnmarshal)
			}
			if payload.State != state || payload.Code != "code-"+provider || payload.Error != "" {
				t.Fatalf("unexpected callback payload: %+v", payload)
			}
		})
	}
}

func TestRequestCodexTokenWithPublicBaseURLReturnsExternalLoginAndCallbackURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authDir := filepath.Join(t.TempDir(), "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("create auth dir: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir, Port: 8317}, nil)
	router := gin.New()
	router.GET("/codex-auth-url", h.RequestCodexToken)

	req := httptest.NewRequest(http.MethodGet, "/codex-auth-url?public_base_url=http%3A%2F%2Fiscdx.duckdns.org%3A8317", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, w.Code, w.Body.String())
	}

	var payload struct {
		URL         string `json:"url"`
		State       string `json:"state"`
		PublicURL   string `json:"public_url"`
		CallbackURL string `json:"callback_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	defer CompleteOAuthSession(payload.State)

	if payload.State == "" {
		t.Fatal("expected state in response")
	}
	wantCallback := "http://iscdx.duckdns.org:8317/codex/callback"
	if payload.CallbackURL != wantCallback {
		t.Fatalf("callback_url = %q, want %q", payload.CallbackURL, wantCallback)
	}
	wantPublicURL := "http://iscdx.duckdns.org:8317/codex/login/" + payload.State
	if payload.PublicURL != wantPublicURL {
		t.Fatalf("public_url = %q, want %q", payload.PublicURL, wantPublicURL)
	}

	parsedAuthURL, errParse := url.Parse(payload.URL)
	if errParse != nil {
		t.Fatalf("parse auth URL: %v", errParse)
	}
	if got := parsedAuthURL.Query().Get("redirect_uri"); got != wantCallback {
		t.Fatalf("redirect_uri = %q, want %q", got, wantCallback)
	}
}

func TestStartPublicCodexDeviceOAuthReturnsDeviceCodeWithoutManagementAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clearPublicCodexDeviceOAuthState()
	t.Cleanup(clearPublicCodexDeviceOAuthState)

	authDir := filepath.Join(t.TempDir(), "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("create auth dir: %v", err)
	}

	originalStart := startCodexDeviceFlow
	originalPoll := pollCodexDeviceFlow
	startCodexDeviceFlow = func(ctx context.Context, cfg *config.Config) (*sdkAuth.CodexDeviceFlow, error) {
		return &sdkAuth.CodexDeviceFlow{
			DeviceAuthID:    "device-auth-id",
			UserCode:        "TEST-CODE",
			VerificationURL: "https://auth.openai.com/codex/device",
			Interval:        time.Second,
		}, nil
	}
	pollCodexDeviceFlow = func(ctx context.Context, cfg *config.Config, flow *sdkAuth.CodexDeviceFlow) (*sdkAuth.CodexDeviceTokenBundle, error) {
		return nil, fmt.Errorf("stop test polling")
	}
	t.Cleanup(func() {
		startCodexDeviceFlow = originalStart
		pollCodexDeviceFlow = originalPoll
	})

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir, Port: 8317}, nil)
	router := gin.New()
	router.POST("/codex-device-start", h.StartPublicCodexDeviceOAuth)

	req := httptest.NewRequest(http.MethodPost, "/codex-device-start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, w.Code, w.Body.String())
	}

	var payload struct {
		Status                  string `json:"status"`
		State                   string `json:"state"`
		UserCode                string `json:"user_code"`
		VerificationURL         string `json:"verification_url"`
		VerificationURLComplete string `json:"verification_url_complete"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	defer CompleteOAuthSession(payload.State)

	if payload.Status != "ok" {
		t.Fatalf("status = %q, want ok", payload.Status)
	}
	if payload.State == "" {
		t.Fatal("expected state in response")
	}
	if payload.UserCode != "TEST-CODE" {
		t.Fatalf("user_code = %q, want TEST-CODE", payload.UserCode)
	}
	if payload.VerificationURL != "https://auth.openai.com/codex/device" {
		t.Fatalf("verification_url = %q", payload.VerificationURL)
	}
	if !strings.Contains(payload.VerificationURLComplete, "user_code=TEST-CODE") {
		t.Fatalf("verification_url_complete = %q, want user_code", payload.VerificationURLComplete)
	}
}

func TestStartPublicCodexDeviceOAuthReusesPendingDeviceCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clearPublicCodexDeviceOAuthState()
	t.Cleanup(clearPublicCodexDeviceOAuthState)

	authDir := filepath.Join(t.TempDir(), "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("create auth dir: %v", err)
	}

	originalStart := startCodexDeviceFlow
	originalPoll := pollCodexDeviceFlow
	startCalls := 0
	releasePoll := make(chan struct{})
	startCodexDeviceFlow = func(ctx context.Context, cfg *config.Config) (*sdkAuth.CodexDeviceFlow, error) {
		startCalls++
		return &sdkAuth.CodexDeviceFlow{
			DeviceAuthID:    "device-auth-id",
			UserCode:        fmt.Sprintf("TEST-CODE-%d", startCalls),
			VerificationURL: "https://auth.openai.com/codex/device",
			Interval:        time.Second,
		}, nil
	}
	pollCodexDeviceFlow = func(ctx context.Context, cfg *config.Config, flow *sdkAuth.CodexDeviceFlow) (*sdkAuth.CodexDeviceTokenBundle, error) {
		select {
		case <-releasePoll:
			return nil, fmt.Errorf("stop test polling")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	t.Cleanup(func() {
		close(releasePoll)
		startCodexDeviceFlow = originalStart
		pollCodexDeviceFlow = originalPoll
	})

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir, Port: 8317}, nil)
	router := gin.New()
	router.POST("/codex-device-start", h.StartPublicCodexDeviceOAuth)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/codex-device-start", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d body=%s", first.Code, http.StatusOK, first.Body.String())
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/codex-device-start", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d body=%s", second.Code, http.StatusOK, second.Body.String())
	}

	var firstPayload, secondPayload struct {
		State    string `json:"state"`
		UserCode string `json:"user_code"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstPayload); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPayload); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	defer CompleteOAuthSession(firstPayload.State)

	if startCalls != 1 {
		t.Fatalf("startCodexDeviceFlow calls = %d, want 1", startCalls)
	}
	if secondPayload.State != firstPayload.State {
		t.Fatalf("second state = %q, want %q", secondPayload.State, firstPayload.State)
	}
	if secondPayload.UserCode != firstPayload.UserCode {
		t.Fatalf("second user_code = %q, want %q", secondPayload.UserCode, firstPayload.UserCode)
	}
}
