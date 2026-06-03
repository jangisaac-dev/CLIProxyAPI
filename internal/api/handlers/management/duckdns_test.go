package management

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newDuckDNSTestHandler(t *testing.T, cfg *config.Config) *Handler {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return &Handler{
		cfg:            cfg,
		configFilePath: configPath,
		failedAttempts: make(map[string]*attemptInfo),
	}
}

func runDuckDNSHandlerRequest(t *testing.T, method string, target string, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}

	handler(ctx)
	return rec
}

func TestDuckDNSSettingsMasksTokenAndPersistsNormalizedDomain(t *testing.T) {
	cfg := &config.Config{
		DuckDNS: config.DuckDNSConfig{
			Domain: "iscdx.duckdns.org",
			Token:  "secret-token-1",
		},
	}
	h := newDuckDNSTestHandler(t, cfg)

	getRec := runDuckDNSHandlerRequest(t, http.MethodGet, "/v0/management/duckdns", "", h.GetDuckDNS)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d body=%s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	if strings.Contains(getRec.Body.String(), "secret-token-1") {
		t.Fatalf("GET response leaked DuckDNS token: %s", getRec.Body.String())
	}

	var getResp struct {
		Domain   string `json:"domain"`
		TokenSet bool   `json:"token_set"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal GET response: %v body=%s", err, getRec.Body.String())
	}
	if getResp.Domain != "iscdx" {
		t.Fatalf("domain = %q, want %q", getResp.Domain, "iscdx")
	}
	if !getResp.TokenSet {
		t.Fatalf("token_set = false, want true")
	}

	patchBody := `{"domain":"https://iscdx.duckdns.org/account","token":"secret-token-2","ip":"203.0.113.10"}`
	patchRec := runDuckDNSHandlerRequest(t, http.MethodPatch, "/v0/management/duckdns", patchBody, h.PatchDuckDNS)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want %d body=%s", patchRec.Code, http.StatusOK, patchRec.Body.String())
	}
	if h.cfg.DuckDNS.Domain != "iscdx" {
		t.Fatalf("stored domain = %q, want %q", h.cfg.DuckDNS.Domain, "iscdx")
	}
	if h.cfg.DuckDNS.Token != "secret-token-2" {
		t.Fatalf("stored token was not updated")
	}
	if h.cfg.DuckDNS.IP != "203.0.113.10" {
		t.Fatalf("stored ip = %q, want %q", h.cfg.DuckDNS.IP, "203.0.113.10")
	}

	configBytes, err := os.ReadFile(h.configFilePath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	persisted := string(configBytes)
	for _, want := range []string{"duckdns:", "domain: iscdx", "token: secret-token-2", "ip: 203.0.113.10"} {
		if !strings.Contains(persisted, want) {
			t.Fatalf("persisted config missing %q:\n%s", want, persisted)
		}
	}
}

func TestPatchDuckDNSSavesDefaultDomainWhenDomainIsBlank(t *testing.T) {
	h := newDuckDNSTestHandler(t, &config.Config{})

	patchBody := `{"domain":"","token":"secret-token","ip":"203.0.113.10"}`
	patchRec := runDuckDNSHandlerRequest(t, http.MethodPatch, "/v0/management/duckdns", patchBody, h.PatchDuckDNS)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want %d body=%s", patchRec.Code, http.StatusOK, patchRec.Body.String())
	}
	if h.cfg.DuckDNS.Domain != "iscdx" {
		t.Fatalf("stored domain = %q, want %q", h.cfg.DuckDNS.Domain, "iscdx")
	}

	configBytes, err := os.ReadFile(h.configFilePath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if persisted := string(configBytes); !strings.Contains(persisted, "domain: iscdx") {
		t.Fatalf("persisted config missing default domain:\n%s", persisted)
	}
}

func TestUpdateDuckDNSCallsOfficialUpdateEndpoint(t *testing.T) {
	var gotQuery string
	oldUpdateURL := duckDNSUpdateURL
	oldFactory := duckDNSHTTPClientFactory
	duckDNSUpdateURL = "https://www.duckdns.org/update"
	duckDNSHTTPClientFactory = func(string) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotQuery = req.URL.RawQuery
			if req.URL.Query().Get("domains") != "iscdx" {
				t.Fatalf("domains query = %q, want %q", req.URL.Query().Get("domains"), "iscdx")
			}
			if req.URL.Query().Get("token") != "secret-token" {
				t.Fatalf("token query = %q, want configured token", req.URL.Query().Get("token"))
			}
			if req.URL.Query().Get("ip") != "203.0.113.10" {
				t.Fatalf("ip query = %q, want %q", req.URL.Query().Get("ip"), "203.0.113.10")
			}
			if req.URL.Query().Get("verbose") != "true" {
				t.Fatalf("verbose query = %q, want true", req.URL.Query().Get("verbose"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("OK 203.0.113.10  UPDATED")),
				Header:     make(http.Header),
			}, nil
		})}
	}
	t.Cleanup(func() {
		duckDNSUpdateURL = oldUpdateURL
		duckDNSHTTPClientFactory = oldFactory
	})

	h := newDuckDNSTestHandler(t, &config.Config{
		DuckDNS: config.DuckDNSConfig{
			Domain: "iscdx.duckdns.org",
			Token:  "secret-token",
			IP:     "203.0.113.10",
		},
	})

	rec := runDuckDNSHandlerRequest(t, http.MethodPost, "/v0/management/duckdns/update", "", h.UpdateDuckDNS)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-token") {
		t.Fatalf("update response leaked DuckDNS token: %s", rec.Body.String())
	}
	if gotQuery == "" {
		t.Fatalf("upstream was not called")
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
		IPv4   string `json:"ipv4"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal update response: %v body=%s", err, rec.Body.String())
	}
	if !resp.OK || resp.Status != "UPDATED" || resp.IPv4 != "203.0.113.10" {
		t.Fatalf("unexpected update response: %+v", resp)
	}
}

func TestUpdateDuckDNSRejectsKOResponse(t *testing.T) {
	oldUpdateURL := duckDNSUpdateURL
	oldFactory := duckDNSHTTPClientFactory
	duckDNSUpdateURL = "https://www.duckdns.org/update"
	duckDNSHTTPClientFactory = func(string) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("KO")),
				Header:     make(http.Header),
			}, nil
		})}
	}
	t.Cleanup(func() {
		duckDNSUpdateURL = oldUpdateURL
		duckDNSHTTPClientFactory = oldFactory
	})

	h := newDuckDNSTestHandler(t, &config.Config{
		DuckDNS: config.DuckDNSConfig{
			Domain: "iscdx",
			Token:  "bad-token",
		},
	})

	rec := runDuckDNSHandlerRequest(t, http.MethodPost, "/v0/management/duckdns/update", "", h.UpdateDuckDNS)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "duckdns_update_failed") {
		t.Fatalf("expected duckdns_update_failed body, got %s", rec.Body.String())
	}
}

func TestGetDuckDNSPublicIPFetchesExternalIPv4(t *testing.T) {
	oldURLs := duckDNSPublicIPLookupURLs
	oldFactory := duckDNSHTTPClientFactory
	duckDNSPublicIPLookupURLs = []string{"https://ip.example.test"}
	duckDNSHTTPClientFactory = func(string) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://ip.example.test" {
				t.Fatalf("lookup url = %q, want %q", req.URL.String(), "https://ip.example.test")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("203.0.113.22\n")),
				Header:     make(http.Header),
			}, nil
		})}
	}
	t.Cleanup(func() {
		duckDNSPublicIPLookupURLs = oldURLs
		duckDNSHTTPClientFactory = oldFactory
	})

	h := newDuckDNSTestHandler(t, &config.Config{})

	rec := runDuckDNSHandlerRequest(t, http.MethodGet, "/v0/management/duckdns/public-ip", "", h.GetDuckDNSPublicIP)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, rec.Body.String())
	}
	if resp.IP != "203.0.113.22" {
		t.Fatalf("ip = %q, want %q", resp.IP, "203.0.113.22")
	}
}
