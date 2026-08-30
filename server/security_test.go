package server

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
)

func TestValidateDownloadURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "public HTTPS", value: "https://example.com/image.qcow2"},
		{name: "loopback", value: "http://127.0.0.1/image.qcow2", wantErr: true},
		{name: "private IPv4", value: "http://10.0.0.1/image.qcow2", wantErr: true},
		{name: "link local IPv6", value: "http://[fe80::1]/image.qcow2", wantErr: true},
		{name: "unsupported scheme", value: "file:///etc/passwd", wantErr: true},
		{name: "unexpected port", value: "https://example.com:8443/image.qcow2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := url.Parse(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			err = validateDownloadURL(parsed)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDownloadURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContentSecurityPolicyIncludesInlineScriptHashes(t *testing.T) {
	script := "window.__flint = true"
	assets := fstest.MapFS{
		"web/out/index.html": {Data: []byte(`<script>` + script + `</script><script src="/app.js"></script>`)},
	}
	sum := sha256.Sum256([]byte(script))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	policy := buildContentSecurityPolicy(assets)
	if !strings.Contains(policy, want) {
		t.Fatalf("CSP %q does not contain %q", policy, want)
	}
}

func TestOpenWebAssetResolvesExportedRoutes(t *testing.T) {
	assets := fstest.MapFS{
		"web/out/vms.html":         {Data: []byte("vms")},
		"web/out/vms/console.html": {Data: []byte("console")},
	}
	file, servedPath, err := openWebAsset(assets, "vms")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if servedPath != "web/out/vms.html" {
		t.Fatalf("served path = %q", servedPath)
	}
	if info, err := fs.Stat(assets, servedPath); err != nil || info.IsDir() {
		t.Fatalf("resolved asset is not a file: %v", err)
	}
}

func TestManifestIsPublicAndHasCorrectContentType(t *testing.T) {
	if !isPublicWebAsset("/site.webmanifest") {
		t.Fatal("web manifest requires authentication")
	}
	if got := webContentType("site.webmanifest"); got != "application/manifest+json" {
		t.Fatalf("manifest content type = %q", got)
	}
}

func TestSameOriginRequest(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		trusted string
		want    bool
	}{
		{name: "same origin", origin: "http://127.0.0.1:5550", want: true},
		{name: "untrusted origin", origin: "https://evil.example", want: false},
		{name: "configured proxy origin", origin: "https://flint.example.com", trusted: "https://flint.example.com", want: true},
		{name: "scheme must match configured origin", origin: "http://flint.example.com", trusted: "https://flint.example.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://127.0.0.1:5550/api/test", nil)
			req.Host = "127.0.0.1:5550"
			req.Header.Set("Origin", tt.origin)
			if got := sameOriginRequest(req, parseTrustedOrigins(tt.trusted)); got != tt.want {
				t.Fatalf("sameOriginRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSameOriginRequestUsesFetchMetadata(t *testing.T) {
	req := httptest.NewRequest("POST", "http://127.0.0.1:5550/api/test", nil)
	req.Host = "unexpected-internal-host:5550"
	req.Header.Set("Origin", "http://127.0.0.1:5550")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if !sameOriginRequest(req, nil) {
		t.Fatal("browser-confirmed same-origin request was rejected")
	}

	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if sameOriginRequest(req, parseTrustedOrigins("http://127.0.0.1:5550")) {
		t.Fatal("browser-confirmed cross-site request was accepted")
	}
}

func TestOriginValidationAppliesOnlyToUnsafeMethods(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		if requestNeedsOriginValidation(method) {
			t.Errorf("%s unexpectedly requires origin validation", method)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if !requestNeedsOriginValidation(method) {
			t.Errorf("%s unexpectedly bypasses origin validation", method)
		}
	}
}

func TestParseTrustedOriginsRejectsNonOrigins(t *testing.T) {
	origins := parseTrustedOrigins("javascript:alert(1),https://user@example.com,https://example.com/path,https://good.example")
	if len(origins) != 1 {
		t.Fatalf("got %d trusted origins, want 1", len(origins))
	}
	if _, ok := origins["https://good.example"]; !ok {
		t.Fatal("valid trusted origin was rejected")
	}
}

func TestValidInterfaceName(t *testing.T) {
	for _, name := range []string{"br0", "eth0.100", "vnet-1"} {
		if !validInterfaceName(name) {
			t.Errorf("valid interface name %q was rejected", name)
		}
	}
	for _, name := range []string{"", "-option", "name with spaces", "interface-name-too-long"} {
		if validInterfaceName(name) {
			t.Errorf("invalid interface name %q was accepted", name)
		}
	}
}
