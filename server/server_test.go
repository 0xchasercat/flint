package server

import (
	"embed"
	"github.com/volantvm/flint/pkg/libvirtclient"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

//go:embed testdata/*
var testAssets embed.FS

func TestServer_GetAPIKey(t *testing.T) {
	// Create a mock client (we'll need to implement a mock for testing)
	// For now, just test that the method exists and returns a string
	client, err := libvirtclient.NewClient("test:///default", "isos", "templates")
	if err != nil {
		t.Skip("Skipping test: libvirt not available in test environment")
	}
	defer client.Close()

	server := NewServer(client, testAssets)
	apiKey := server.GetAPIKey()

	// API key should be a non-empty string
	if apiKey == "" {
		t.Error("GetAPIKey() returned empty string")
	}

	// API key should be 64 characters (32 bytes hex encoded)
	if len(apiKey) != 64 {
		t.Errorf("GetAPIKey() returned string of length %d, expected 64", len(apiKey))
	}
}

func TestConsoleTokenIsBoundAndSingleUse(t *testing.T) {
	s := &Server{consoleTokens: make(map[string]consoleToken)}
	token, err := s.issueConsoleToken("vm-1", "serial")
	if err != nil {
		t.Fatal(err)
	}
	if s.consumeConsoleToken(token, "vm-2", "serial") {
		t.Fatal("token accepted for another VM")
	}

	token, err = s.issueConsoleToken("vm-1", "serial")
	if err != nil {
		t.Fatal(err)
	}
	if !s.consumeConsoleToken(token, "vm-1", "serial") {
		t.Fatal("valid token rejected")
	}
	if s.consumeConsoleToken(token, "vm-1", "serial") {
		t.Fatal("one-time token was accepted twice")
	}

	expiredToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	s.consoleTokens[expiredToken] = consoleToken{VMUUID: "vm-1", Kind: "serial", Expiry: time.Now().Add(-time.Second)}
	if s.consumeConsoleToken(expiredToken, "vm-1", "serial") {
		t.Fatal("expired token accepted")
	}
}

func TestConsoleMetadataRequiresAuthentication(t *testing.T) {
	client, err := libvirtclient.NewClient("test:///default", "isos", "templates")
	if err != nil {
		t.Skip("libvirt test driver unavailable")
	}
	defer client.Close()
	s := NewServer(client, testAssets)
	req := httptest.NewRequest(http.MethodGet, "/api/vms/550e8400-e29b-41d4-a716-446655440000/serial-console", nil)
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestValidateAuthToken(t *testing.T) {
	// Create a test server
	client, err := libvirtclient.NewClient("test:///default", "isos", "templates")
	if err != nil {
		t.Skip("Skipping test: libvirt not available in test environment")
	}
	defer client.Close()

	server := NewServer(client, testAssets)

	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{
			name:  "valid token",
			token: server.GetAPIKey(),
			valid: true,
		},
		{
			name:  "empty token",
			token: "",
			valid: false,
		},
		{
			name:  "wrong length",
			token: "short",
			valid: false,
		},
		{
			name:  "invalid hex",
			token: "zzzz8400e29b41d4a716446655440000e29b41d4a716446655440000e29b41d4a716",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.validateAuthToken(tt.token)
			if result != tt.valid {
				t.Errorf("validateAuthToken() = %v, want %v", result, tt.valid)
			}
		})
	}
}

func TestRateLimiterDoesNotThrottleStaticAssets(t *testing.T) {
	s := &Server{rateLimiters: make(map[string]*rateLimiter)}
	handler := s.rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 250; i++ {
		req := httptest.NewRequest(http.MethodGet, "/_next/static/app.js", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("static request %d returned %d", i+1, response.Code)
		}
	}
	if len(s.rateLimiters) != 0 {
		t.Fatalf("static requests created %d rate limiters", len(s.rateLimiters))
	}
}
