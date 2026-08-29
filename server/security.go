package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	consoleTokenTTL = 60 * time.Second
	maxDownloadSize = int64(10 << 30)
)

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func sameOriginRequest(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func sameWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func (s *Server) requestSizeLimitMiddleware(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) issueConsoleToken(vmUUID, kind string) (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	token := hex.EncodeToString(random)

	s.consoleTokensMu.Lock()
	defer s.consoleTokensMu.Unlock()
	now := time.Now()
	for value, entry := range s.consoleTokens {
		if now.After(entry.Expiry) {
			delete(s.consoleTokens, value)
		}
	}
	s.consoleTokens[token] = consoleToken{VMUUID: vmUUID, Kind: kind, Expiry: now.Add(consoleTokenTTL)}
	return token, nil
}

func (s *Server) consumeConsoleToken(token, vmUUID, kind string) bool {
	if len(token) != 64 {
		return false
	}
	s.consoleTokensMu.Lock()
	defer s.consoleTokensMu.Unlock()
	entry, ok := s.consoleTokens[token]
	if !ok {
		return false
	}
	delete(s.consoleTokens, token)
	return time.Now().Before(entry.Expiry) && entry.VMUUID == vmUUID && entry.Kind == kind
}

func safeHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isPublicIP(ip) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				}
			}
			return nil, fmt.Errorf("destination resolves only to private or reserved addresses")
		},
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Minute}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return validateDownloadURL(req.URL)
	}
	return client
}

func validateDownloadURL(value *url.URL) error {
	if value == nil || (value.Scheme != "https" && value.Scheme != "http") || value.Hostname() == "" {
		return fmt.Errorf("only absolute HTTP and HTTPS URLs are allowed")
	}
	port := value.Port()
	if port != "" && port != "80" && port != "443" {
		return fmt.Errorf("only ports 80 and 443 are allowed")
	}
	if ip := net.ParseIP(value.Hostname()); ip != nil && !isPublicIP(ip) {
		return fmt.Errorf("private and reserved destinations are not allowed")
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified()
}
