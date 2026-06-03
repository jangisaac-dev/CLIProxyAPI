package api

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

func normalizeHTTPServeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func normalizeListenerError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (s *Server) acceptMuxConnections(listener net.Listener, httpListener *muxListener) error {
	if s == nil || listener == nil {
		return net.ErrClosed
	}

	for {
		conn, errAccept := listener.Accept()
		if errAccept != nil {
			return errAccept
		}
		if conn == nil {
			continue
		}

		// Dispatch each connection to a goroutine so that slow/idle clients
		// cannot block the accept loop. Previously, TLS handshake and
		// reader.Peek(1) were performed inline; an idle TCP connection that
		// never sent bytes would block Peek indefinitely, preventing all
		// subsequent connections from being accepted (issue #3267).
		go s.routeMuxConnection(conn, httpListener)
	}
}

// routeMuxConnection performs per-connection protocol detection and routing.
func (s *Server) routeMuxConnection(conn net.Conn, httpListener *muxListener) {
	// Set a read deadline so that idle connections that never send bytes do not
	// leak goroutines and file descriptors. The deadline is cleared once the
	// connection is successfully routed to its handler.
	const muxSniffDeadline = 10 * time.Second
	_ = conn.SetReadDeadline(time.Now().Add(muxSniffDeadline))

	tlsConn, ok := conn.(*tls.Conn)
	if ok {
		s.routeTLSMuxConnection(tlsConn, httpListener)
		return
	}

	reader := bufio.NewReader(conn)
	prefix, errPeek := reader.Peek(1)
	if errPeek != nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection after protocol peek failure: %v", errClose)
		}
		return
	}

	if isTLSHandshakePrefix(prefix[0]) {
		tlsConfig := s.muxTLSConfig()
		if tlsConfig != nil {
			s.routeTLSMuxConnection(tls.Server(&bufferedConn{Conn: conn, reader: reader}, tlsConfig), httpListener)
			return
		}
	}

	if isRedisRESPPrefix(prefix[0]) {
		_ = conn.SetReadDeadline(time.Time{})
		s.handleRedisConnection(conn, reader)
		return
	}

	if httpListener == nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection without HTTP listener: %v", errClose)
		}
		return
	}

	if errPut := httpListener.Put(&bufferedConn{Conn: conn, reader: reader}); errPut != nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection after HTTP routing failure: %v", errClose)
		}
	} else {
		_ = conn.SetReadDeadline(time.Time{})
	}
}

func (s *Server) muxTLSConfig() *tls.Config {
	if s == nil || s.server == nil || s.server.TLSConfig == nil {
		return nil
	}
	return s.server.TLSConfig
}

func isTLSHandshakePrefix(prefix byte) bool {
	return prefix == 0x16
}

func (s *Server) routeTLSMuxConnection(tlsConn *tls.Conn, httpListener *muxListener) {
	if tlsConn == nil {
		return
	}
	if errHandshake := tlsConn.Handshake(); errHandshake != nil {
		if errClose := tlsConn.Close(); errClose != nil {
			log.Errorf("failed to close connection after TLS handshake error: %v", errClose)
		}
		return
	}
	proto := strings.TrimSpace(tlsConn.ConnectionState().NegotiatedProtocol)
	if proto != "" && proto != "h2" && proto != "http/1.1" {
		if errClose := tlsConn.Close(); errClose != nil {
			log.Errorf("failed to close connection for unsupported TLS protocol: %v", errClose)
		}
		return
	}
	if httpListener == nil {
		if errClose := tlsConn.Close(); errClose != nil {
			log.Errorf("failed to close TLS connection without HTTP listener: %v", errClose)
		}
		return
	}
	if errPut := httpListener.Put(tlsConn); errPut != nil {
		if errClose := tlsConn.Close(); errClose != nil {
			log.Errorf("failed to close connection after HTTP routing failure: %v", errClose)
		}
		return
	}
	_ = tlsConn.SetReadDeadline(time.Time{})
}
