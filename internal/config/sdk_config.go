// Package config provides configuration management for the CLI Proxy API server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// SDKConfig represents the application's configuration, loaded from a YAML file.
type SDKConfig struct {
	// ProxyURL is the URL of an optional proxy server to use for outbound requests.
	// This is the effective value used at runtime. It is kept in sync with ProxySettings when
	// the structured settings are updated via management API.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// ProxySettings provides a structured way to configure the global outbound proxy
	// (protocol, host, port, credentials) with an enabled flag. This is primarily
	// controlled via the management API (/v0/management/config/proxy-settings) so that
	// on/off and different proxy types (http/https/socks5) can be toggled at runtime
	// from Web-UI or external shortcuts without editing the file manually.
	ProxySettings ProxySettings `yaml:"proxy-settings,omitempty" json:"proxy-settings,omitempty"`

	// DisableImageGeneration controls whether the built-in image_generation tool is injected/allowed.
	//
	// Supported values:
	//   - false (default): image_generation is enabled everywhere (normal behavior).
	//   - true: image_generation is disabled everywhere. The server stops injecting it, removes it from request payloads,
	//     and returns 404 for /v1/images/generations and /v1/images/edits.
	//   - "chat": disable image_generation injection for all non-images endpoints (e.g. /v1/responses, /v1/chat/completions),
	//     while keeping /v1/images/generations and /v1/images/edits enabled and preserving image_generation there.
	DisableImageGeneration DisableImageGenerationMode `yaml:"disable-image-generation" json:"disable-image-generation"`

	// GPTImage2BaseModel sets the base (mainline) model used when proxying GPT Image 2
	// requests via the hosted image_generation tool (e.g. Codex OAuth /v1/images/*).
	//
	// The value must start with "gpt-" (case-insensitive). If empty or invalid, the
	// default base model ("gpt-5.4-mini") is used.
	GPTImage2BaseModel string `yaml:"gpt-image-2-base-model,omitempty" json:"gpt-image-2-base-model,omitempty"`

	// EnableGeminiCLIEndpoint controls whether Gemini CLI internal endpoints (/v1internal:*) are enabled.
	// Default is false for safety; when false, /v1internal:* requests are rejected.
	EnableGeminiCLIEndpoint bool `yaml:"enable-gemini-cli-endpoint" json:"enable-gemini-cli-endpoint"`

	// ForceModelPrefix requires explicit model prefixes (e.g., "teamA/gemini-3-pro-preview")
	// to target prefixed credentials. When false, unprefixed model requests may use prefixed
	// credentials as well.
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog enables or disables detailed request logging functionality.
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// APIKeys is a list of keys for authenticating clients to this proxy server.
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// PassthroughHeaders controls whether upstream response headers are forwarded to downstream clients.
	// Default is false (disabled).
	PassthroughHeaders bool `yaml:"passthrough-headers" json:"passthrough-headers"`

	// Streaming configures server-side streaming behavior (keep-alives and safe bootstrap retries).
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// NonStreamKeepAliveInterval controls how often blank lines are emitted for non-streaming responses.
	// <= 0 disables keep-alives. Value is in seconds.
	NonStreamKeepAliveInterval int `yaml:"nonstream-keepalive-interval,omitempty" json:"nonstream-keepalive-interval,omitempty"`
}

// StreamingConfig holds server streaming behavior configuration.
type StreamingConfig struct {
	// KeepAliveSeconds controls how often the server emits SSE heartbeats (": keep-alive\n\n").
	// <= 0 disables keep-alives. Default is 0.
	KeepAliveSeconds int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries controls how many times the server may retry a streaming request before any bytes are sent,
	// to allow auth rotation / transient recovery.
	// <= 0 disables bootstrap retries. Default is 0.
	BootstrapRetries int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}

// ProxySettings holds structured global proxy configuration.
// It supports on/off toggle and common protocols (http, https, socks5, socks5h).
// The management API can update this fully or partially (e.g. only toggle enabled,
// or only change host+port). After update the effective ProxyURL string is
// recomputed so that all runtime code paths (including the recently fixed uTLS
// Claude paths) immediately see the change.
type ProxySettings struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty"` // "http", "https", "socks5", "socks5h"
	Host     string `yaml:"host,omitempty" json:"host,omitempty"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

// EffectiveProxyURL returns the proxy URL string that should be used at runtime,
// computed from ProxySettings if it is enabled, otherwise falling back to the
// legacy ProxyURL field. When ProxySettings is enabled=false we return "direct"
// so that no proxy (not even environment proxies) is used — matching the
// documented semantics of the "direct" value.
func (s *SDKConfig) EffectiveProxyURL() string {
	ps := s.ProxySettings
	if ps.Enabled {
		if ps.Host == "" {
			return ""
		}
		protocol := strings.ToLower(strings.TrimSpace(ps.Protocol))
		if protocol == "" {
			protocol = "http"
		}
		var auth string
		if ps.Username != "" {
			if ps.Password != "" {
				auth = url.QueryEscape(ps.Username) + ":" + url.QueryEscape(ps.Password) + "@"
			} else {
				auth = url.QueryEscape(ps.Username) + "@"
			}
		}
		port := ps.Port
		if port == 0 {
			if protocol == "https" || protocol == "socks5h" {
				port = 443
			} else {
				port = 1080 // common default for socks, 8080 for http is also common but we pick a reasonable one
				if protocol == "http" || protocol == "https" {
					port = 8080
				}
			}
		}
		return fmt.Sprintf("%s://%s%s:%d", protocol, auth, ps.Host, port)
	}
	// When structured settings are present but disabled, force direct.
	// If no structured settings, fall back to the legacy plain ProxyURL string.
	if ps.Host != "" || ps.Protocol != "" {
		return "direct"
	}
	return s.ProxyURL
}
