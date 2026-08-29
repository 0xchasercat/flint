package server

import (
	"net/url"
	"testing"
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
