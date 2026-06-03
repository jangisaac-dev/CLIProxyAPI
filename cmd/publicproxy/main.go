package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var (
		httpAddr     string
		httpsAddr    string
		targetRaw    string
		certFile     string
		keyFile      string
		challengeDir string
		httpMode     string
	)
	flag.StringVar(&httpAddr, "http", ":80", "HTTP listen address")
	flag.StringVar(&httpsAddr, "https", ":443", "HTTPS listen address")
	flag.StringVar(&targetRaw, "target", "http://127.0.0.1:8317", "upstream target URL")
	flag.StringVar(&certFile, "cert", "", "TLS certificate file")
	flag.StringVar(&keyFile, "key", "", "TLS private key file")
	flag.StringVar(&challengeDir, "challenge-dir", "", "ACME HTTP-01 challenge directory")
	flag.StringVar(&httpMode, "http-mode", "redirect", "HTTP mode: redirect or proxy")
	flag.Parse()

	target, err := url.Parse(targetRaw)
	if err != nil {
		exitf("invalid target: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, fmt.Sprintf("upstream unavailable: %v", err), http.StatusBadGateway)
	}
	proxy.Director = directorWithForwardedHeaders(proxy.Director, target)

	errCh := make(chan error, 2)
	if httpAddr != "" {
		go func() {
			errCh <- serveHTTP(httpAddr, challengeDir, httpMode, proxy)
		}()
	}
	if httpsAddr != "" {
		go func() {
			errCh <- serveHTTPS(httpsAddr, certFile, keyFile, proxy)
		}()
	}

	err = <-errCh
	exitf("%v", err)
}

func serveHTTP(addr string, challengeDir string, mode string, proxy http.Handler) error {
	mux := http.NewServeMux()
	if strings.TrimSpace(challengeDir) != "" {
		mux.HandleFunc("/.well-known/acme-challenge/", func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
			if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.TrimSpace(name) == "" {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(challengeDir, name))
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(mode), "proxy") {
			proxy.ServeHTTP(w, r)
			return
		}
		host := stripPort(r.Host)
		location := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, location, http.StatusMovedPermanently)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return server.ListenAndServe()
}

func serveHTTPS(addr string, certFile string, keyFile string, handler http.Handler) error {
	if strings.TrimSpace(certFile) == "" || strings.TrimSpace(keyFile) == "" {
		return errors.New("cert and key are required for HTTPS")
	}
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"h2", "http/1.1"},
		},
	}
	return server.ListenAndServeTLS(certFile, keyFile)
}

func directorWithForwardedHeaders(next func(*http.Request), target *url.URL) func(*http.Request) {
	return func(req *http.Request) {
		originalHost := req.Host
		next(req)
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Host", originalHost)
		proto := "http"
		if req.TLS != nil {
			proto = "https"
		}
		req.Header.Set("X-Forwarded-Proto", proto)
		if ip, _, err := net.SplitHostPort(req.RemoteAddr); err == nil && ip != "" {
			appendHeader(req.Header, "X-Forwarded-For", ip)
		}
	}
}

func appendHeader(header http.Header, key string, value string) {
	if existing := header.Get(key); existing != "" {
		header.Set(key, existing+", "+value)
		return
	}
	header.Set(key, value)
}

func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
