package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	inlineScriptPattern = regexp.MustCompile(`(?is)<script([^>]*)>(.*?)</script>`)
	scriptSourcePattern = regexp.MustCompile(`(?i)\bsrc\s*=`)
)

const (
	consoleTokenTTL = 60 * time.Second
	maxDownloadSize = int64(10 << 30)
)

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func requestNeedsOriginValidation(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func sameOriginRequest(r *http.Request, trustedOrigins map[string]struct{}) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "same-origin":
		// Sec-Fetch-Site is a browser-controlled forbidden request header, so
		// page JavaScript cannot forge this value for a cross-origin request.
		return true
	case "cross-site":
		return false
	}

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
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	_, trusted := trustedOrigins[canonicalOrigin(parsed)]
	return trusted
}

func sameWebSocketOrigin(r *http.Request, trustedOrigins map[string]struct{}) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	_, trusted := trustedOrigins[canonicalOrigin(parsed)]
	return trusted
}

func parseTrustedOrigins(value string) map[string]struct{} {
	origins := make(map[string]struct{})
	for _, entry := range strings.Split(value, ",") {
		parsed, err := url.Parse(strings.TrimSpace(entry))
		if err != nil || parsed.Host == "" || parsed.User != nil ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			continue
		}
		origins[canonicalOrigin(parsed)] = struct{}{}
	}
	return origins
}

func canonicalOrigin(origin *url.URL) string {
	return strings.ToLower(origin.Scheme) + "://" + strings.ToLower(origin.Host)
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
		w.Header().Set("Content-Security-Policy", s.contentSecurityPolicy)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func buildContentSecurityPolicy(assets fs.FS) string {
	hashes := make(map[string]struct{})
	_ = fs.WalkDir(assets, "web/out", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || path.Ext(assetPath) != ".html" {
			return nil
		}
		content, err := fs.ReadFile(assets, assetPath)
		if err != nil {
			return nil
		}
		for _, match := range inlineScriptPattern.FindAllSubmatch(content, -1) {
			if scriptSourcePattern.Match(match[1]) || len(match[2]) == 0 {
				continue
			}
			sum := sha256.Sum256(match[2])
			hashes["'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'"] = struct{}{}
		}
		return nil
	})

	scriptSources := []string{"'self'"}
	for hash := range hashes {
		scriptSources = append(scriptSources, hash)
	}
	sort.Strings(scriptSources[1:])
	return "default-src 'self'; script-src " + strings.Join(scriptSources, " ") + "; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
}

func isPublicWebAsset(requestPath string) bool {
	if strings.HasPrefix(requestPath, "/_next/") {
		return true
	}
	switch strings.ToLower(path.Ext(requestPath)) {
	case ".css", ".js", ".map", ".woff", ".woff2", ".png", ".svg", ".ico", ".webmanifest":
		return true
	default:
		return false
	}
}

func openWebAsset(assets fs.FS, requestPath string) (fs.File, string, error) {
	candidates := []string{"web/out/" + requestPath, "web/public/" + requestPath}
	if path.Ext(requestPath) == "" {
		candidates = []string{"web/out/" + requestPath + ".html", "web/out/" + requestPath + "/index.html"}
	}
	for _, candidate := range candidates {
		file, err := assets.Open(candidate)
		if err != nil {
			continue
		}
		info, err := file.Stat()
		if err == nil && !info.IsDir() {
			return file, candidate, nil
		}
		file.Close()
	}
	return nil, "", fs.ErrNotExist
}

func webContentType(assetPath string) string {
	switch strings.ToLower(path.Ext(assetPath)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css; charset=utf-8"
	case ".webmanifest":
		return "application/manifest+json"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	}
	if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
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
