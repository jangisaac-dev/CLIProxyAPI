package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRequestCodexDeviceUserCodeClassifiesCloudflareChallenge(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"cf-mitigated": []string{"challenge"},
				},
				Body: io.NopCloser(strings.NewReader("<!doctype html><title>Just a moment...</title>")),
			}, nil
		}),
	}

	_, err := requestCodexDeviceUserCode(context.Background(), client)
	if err == nil {
		t.Fatal("requestCodexDeviceUserCode() error = nil, want challenge error")
	}
	if !IsCodexDeviceAuthChallenge(err) {
		t.Fatalf("IsCodexDeviceAuthChallenge() = false for %v", err)
	}
	if strings.Contains(err.Error(), "<!doctype html>") {
		t.Fatalf("error leaked challenge HTML: %v", err)
	}
}
