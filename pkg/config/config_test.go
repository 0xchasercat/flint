package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	cfg := DefaultConfig()
	cfg.APIKey = "test-key"
	if err := cfg.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("config directory permissions = %o, want 700", got)
	}
}

func TestBindEnvironmentOverridesServerAddress(t *testing.T) {
	t.Setenv("FLINT_SERVER_HOST", "192.0.2.1")
	t.Setenv("FLINT_SERVER_PORT", "5551")
	t.Setenv("FLINT_BIND_ADDRESS", "::1")
	t.Setenv("FLINT_BIND_PORT", "8080")

	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetServerAddress(); got != "[::1]:8080" {
		t.Fatalf("server address = %q, want %q", got, "[::1]:8080")
	}
}

func TestLegacyServerEnvironmentStillWorks(t *testing.T) {
	t.Setenv("FLINT_SERVER_HOST", "0.0.0.0")
	t.Setenv("FLINT_SERVER_PORT", "5552")

	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetServerAddress(); got != "0.0.0.0:5552" {
		t.Fatalf("server address = %q, want %q", got, "0.0.0.0:5552")
	}
}

func TestInvalidBindPortFailsValidation(t *testing.T) {
	t.Setenv("FLINT_BIND_PORT", "not-a-port")
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid FLINT_BIND_PORT passed validation")
	}
}
