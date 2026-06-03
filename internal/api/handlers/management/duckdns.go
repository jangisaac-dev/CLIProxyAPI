package management

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	log "github.com/sirupsen/logrus"
)

const (
	defaultDuckDNSDomain = "iscdx"
	duckDNSUserAgent     = "CLIProxyAPI-DuckDNS"
)

var (
	duckDNSUpdateURL           = "https://www.duckdns.org/update"
	duckDNSHTTPClientFactory   = newDuckDNSHTTPClient
	duckDNSUpdateResponseLimit = int64(2048)
	duckDNSPublicIPLookupURLs  = []string{
		"https://api.ipify.org",
		"https://checkip.amazonaws.com",
	}
)

type duckDNSPatchBody struct {
	Domain *string `json:"domain,omitempty"`
	Token  *string `json:"token,omitempty"`
	IP     *string `json:"ip,omitempty"`
}

type duckDNSConfigResponse struct {
	Domain       string `json:"domain"`
	Hostname     string `json:"hostname,omitempty"`
	IP           string `json:"ip,omitempty"`
	TokenSet     bool   `json:"token_set"`
	TokenPreview string `json:"token_preview,omitempty"`
}

type duckDNSUpdateResponse struct {
	OK     bool   `json:"ok"`
	Status string `json:"status,omitempty"`
	IPv4   string `json:"ipv4,omitempty"`
	IPv6   string `json:"ipv6,omitempty"`
}

type duckDNSPublicIPResponse struct {
	IP string `json:"ip"`
}

// GetDuckDNS returns DuckDNS settings without exposing the token value.
func (h *Handler) GetDuckDNS(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusOK, duckDNSConfigResponse{})
		return
	}

	h.mu.Lock()
	cfg := h.cfg.DuckDNS
	h.mu.Unlock()

	c.JSON(http.StatusOK, newDuckDNSConfigResponse(cfg))
}

// PatchDuckDNS updates DuckDNS settings and persists them to config.yaml.
func (h *Handler) PatchDuckDNS(c *gin.Context) {
	var body duckDNSPatchBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": err.Error()})
		return
	}

	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config_unavailable"})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config_unavailable"})
		return
	}

	if body.Domain != nil {
		domain, err := normalizeDuckDNSDomain(*body.Domain)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_domain", "message": err.Error()})
			return
		}
		if domain == "" {
			domain = defaultDuckDNSDomain
		}
		h.cfg.DuckDNS.Domain = domain
	} else if strings.TrimSpace(h.cfg.DuckDNS.Domain) == "" {
		h.cfg.DuckDNS.Domain = defaultDuckDNSDomain
	}
	if body.Token != nil {
		h.cfg.DuckDNS.Token = strings.TrimSpace(*body.Token)
	}
	if body.IP != nil {
		ip, err := normalizeDuckDNSIPv4(*body.IP)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ip", "message": err.Error()})
			return
		}
		h.cfg.DuckDNS.IP = ip
	}

	if err := config.SaveConfigPreserveComments(h.configFilePath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save_failed", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, newDuckDNSConfigResponse(h.cfg.DuckDNS))
}

// UpdateDuckDNS calls DuckDNS to update the configured domain to the configured IP.
func (h *Handler) UpdateDuckDNS(c *gin.Context) {
	cfg, proxyURL := h.duckDNSConfigSnapshot()

	var body duckDNSPatchBody
	if c.Request != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": err.Error()})
			return
		}
	}

	if body.Domain != nil {
		domain, err := normalizeDuckDNSDomain(*body.Domain)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_domain", "message": err.Error()})
			return
		}
		cfg.Domain = domain
	}
	if body.Token != nil {
		cfg.Token = strings.TrimSpace(*body.Token)
	}
	if body.IP != nil {
		ip, err := normalizeDuckDNSIPv4(*body.IP)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ip", "message": err.Error()})
			return
		}
		cfg.IP = ip
	}

	domain, err := normalizeDuckDNSDomain(cfg.Domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_domain", "message": err.Error()})
		return
	}
	if domain == "" {
		domain = defaultDuckDNSDomain
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_token"})
		return
	}
	ip, err := normalizeDuckDNSIPv4(cfg.IP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ip", "message": err.Error()})
		return
	}

	updateURL, err := buildDuckDNSUpdateURL(domain, token, ip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "request_create_failed", "message": err.Error()})
		return
	}

	client := duckDNSHTTPClientFactory(proxyURL)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, updateURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "request_create_failed", "message": err.Error()})
		return
	}
	req.Header.Set("User-Agent", duckDNSUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "request_failed", "message": err.Error()})
		return
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.WithError(errClose).Debug("failed to close DuckDNS response body")
		}
	}()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, duckDNSUpdateResponseLimit))
	bodyText := strings.TrimSpace(string(respBody))
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "unexpected_status", "message": fmt.Sprintf("status %d: %s", resp.StatusCode, bodyText)})
		return
	}

	result, ok := parseDuckDNSUpdateResponse(bodyText)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "duckdns_update_failed", "message": bodyText})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetDuckDNSPublicIP returns the server's current public IPv4 address.
func (h *Handler) GetDuckDNSPublicIP(c *gin.Context) {
	_, proxyURL := h.duckDNSConfigSnapshot()

	ip, err := lookupDuckDNSPublicIPv4(c.Request.Context(), proxyURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "public_ip_lookup_failed", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, duckDNSPublicIPResponse{IP: ip})
}

func (h *Handler) duckDNSConfigSnapshot() (config.DuckDNSConfig, string) {
	if h == nil || h.cfg == nil {
		return config.DuckDNSConfig{}, ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cfg.DuckDNS, strings.TrimSpace(h.cfg.ProxyURL)
}

func newDuckDNSConfigResponse(cfg config.DuckDNSConfig) duckDNSConfigResponse {
	domain, err := normalizeDuckDNSDomain(cfg.Domain)
	if err != nil {
		domain = strings.TrimSpace(cfg.Domain)
	}
	if domain == "" {
		domain = defaultDuckDNSDomain
	}
	hostname := ""
	if domain != "" {
		hostname = domain + ".duckdns.org"
	}
	token := strings.TrimSpace(cfg.Token)
	return duckDNSConfigResponse{
		Domain:       domain,
		Hostname:     hostname,
		IP:           strings.TrimSpace(cfg.IP),
		TokenSet:     token != "",
		TokenPreview: previewDuckDNSToken(token),
	}
}

func previewDuckDNSToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "set"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func normalizeDuckDNSDomain(raw string) (string, error) {
	domain := strings.TrimSpace(raw)
	if domain == "" {
		return "", nil
	}
	if strings.Contains(domain, "://") {
		parsed, err := url.Parse(domain)
		if err != nil {
			return "", err
		}
		domain = parsed.Host
	}
	domain = strings.TrimPrefix(domain, "//")
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = host
	} else if strings.Count(domain, ":") == 1 {
		domain = strings.SplitN(domain, ":", 2)[0]
	}
	for _, separator := range []string{"/", "?", "#"} {
		if idx := strings.Index(domain, separator); idx >= 0 {
			domain = domain[:idx]
		}
	}
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	domain = strings.TrimSuffix(domain, ".duckdns.org")
	if domain == "" {
		return "", nil
	}
	if len(domain) > 63 {
		return "", fmt.Errorf("domain label is too long")
	}
	if strings.HasPrefix(domain, "-") || strings.HasSuffix(domain, "-") {
		return "", fmt.Errorf("domain label cannot start or end with '-'")
	}
	for _, r := range domain {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return "", fmt.Errorf("domain label contains unsupported character %q", r)
	}
	return domain, nil
}

func normalizeDuckDNSIPv4(raw string) (string, error) {
	ip := strings.TrimSpace(raw)
	if ip == "" {
		return "", nil
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return "", fmt.Errorf("ip must be an IPv4 address or empty")
	}
	return parsed.String(), nil
}

func buildDuckDNSUpdateURL(domain string, token string, ip string) (string, error) {
	updateURL, err := url.Parse(duckDNSUpdateURL)
	if err != nil {
		return "", err
	}
	q := updateURL.Query()
	q.Set("domains", domain)
	q.Set("token", token)
	q.Set("verbose", "true")
	if ip != "" {
		q.Set("ip", ip)
	}
	updateURL.RawQuery = q.Encode()
	return updateURL.String(), nil
}

func newDuckDNSHTTPClient(proxyURL string) *http.Client {
	client := &http.Client{}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL != "" {
		util.SetProxy(&sdkconfig.SDKConfig{ProxyURL: proxyURL}, client)
	}
	return client
}

func lookupDuckDNSPublicIPv4(ctx context.Context, proxyURL string) (string, error) {
	var lastErr error
	client := duckDNSHTTPClientFactory(proxyURL)
	for _, lookupURL := range duckDNSPublicIPLookupURLs {
		lookupURL = strings.TrimSpace(lookupURL)
		if lookupURL == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", duckDNSUserAgent)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		if errClose := resp.Body.Close(); errClose != nil {
			log.WithError(errClose).Debug("failed to close public IP response body")
		}
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d from %s", resp.StatusCode, lookupURL)
			continue
		}
		ip, err := normalizeDuckDNSIPv4(string(body))
		if err != nil {
			lastErr = err
			continue
		}
		return ip, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no public IP lookup endpoints configured")
}

func parseDuckDNSUpdateResponse(raw string) (duckDNSUpdateResponse, bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 || fields[0] != "OK" {
		return duckDNSUpdateResponse{}, false
	}

	result := duckDNSUpdateResponse{OK: true}
	if len(fields) > 1 {
		result.IPv4 = fields[1]
	}
	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "UPDATED", "NOCHANGE":
			result.Status = fields[i]
		}
	}
	if len(fields) > 3 {
		result.IPv6 = fields[2]
	}
	return result, true
}
