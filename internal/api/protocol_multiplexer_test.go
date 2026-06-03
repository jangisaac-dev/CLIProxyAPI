package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcceptMuxNotBlockedByIdleConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	var routed atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routed.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	httpServer := &http.Server{Handler: handler}
	defer httpServer.Close()

	muxLn := newMuxListener(listener.Addr(), 1024)
	server := &Server{managementRoutesEnabled: atomic.Bool{}}
	server.managementRoutesEnabled.Store(false)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.acceptMuxConnections(listener, muxLn)
	}()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- httpServer.Serve(muxLn)
	}()

	// Open an idle TCP connection that never sends any bytes.
	idleConn, err := net.DialTimeout("tcp", listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial idle connection: %v", err)
	}
	defer idleConn.Close()

	// Give the accept loop time to pick up the idle connection.
	time.Sleep(50 * time.Millisecond)

	// Send a real HTTP request. Before the fix, the accept loop would be
	// blocked on Peek(1) for the idle connection, causing this request to
	// time out.
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		listener.Close()
		t.Fatalf("HTTP request failed (accept loop may be blocked by idle connection): %v", err)
	}
	resp.Body.Close()

	listener.Close()

	if routed.Load() == 0 {
		t.Error("expected at least one request to be routed")
	}

	httpServer.Close()
	muxLn.Close()
	select {
	case err := <-serveErrCh:
		if normalizeHTTPServeError(err) != nil {
			t.Fatalf("HTTP server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP server did not stop")
	}
}

func TestAcceptMuxRoutesPlainHTTPAndTLSOnSamePort(t *testing.T) {
	tlsConfig := testMuxTLSConfig(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	httpServer := &http.Server{Handler: handler, TLSConfig: tlsConfig}
	defer httpServer.Close()

	muxLn := newMuxListener(listener.Addr(), 1024)
	server := &Server{server: httpServer}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.acceptMuxConnections(listener, muxLn)
	}()
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- httpServer.Serve(muxLn)
	}()

	plainClient := &http.Client{Timeout: 3 * time.Second}
	plainResp, err := plainClient.Get("http://" + listener.Addr().String() + "/plain")
	if err != nil {
		t.Fatalf("plain HTTP request failed: %v", err)
	}
	plainBody, err := io.ReadAll(plainResp.Body)
	plainResp.Body.Close()
	if err != nil {
		t.Fatalf("failed to read plain response body: %v", err)
	}
	if string(plainBody) != "/plain" {
		t.Fatalf("plain response body = %q, want /plain", plainBody)
	}

	tlsClient := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	tlsResp, err := tlsClient.Get("https://" + listener.Addr().String() + "/tls")
	if err != nil {
		t.Fatalf("TLS request failed: %v", err)
	}
	tlsBody, err := io.ReadAll(tlsResp.Body)
	tlsResp.Body.Close()
	if err != nil {
		t.Fatalf("failed to read TLS response body: %v", err)
	}
	if string(tlsBody) != "/tls" {
		t.Fatalf("TLS response body = %q, want /tls", tlsBody)
	}

	listener.Close()
	httpServer.Close()
	muxLn.Close()

	select {
	case err := <-errCh:
		if normalizeListenerError(err) != nil {
			t.Fatalf("mux accept returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("mux accept did not stop")
	}
	select {
	case err := <-serveErrCh:
		if normalizeHTTPServeError(err) != nil {
			t.Fatalf("HTTP server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP server did not stop")
	}
}

func testMuxTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create test certificate: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{der},
			PrivateKey:  privateKey,
		}},
		NextProtos: []string{"http/1.1"},
	}
}
