package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPassphraseFromEnvironmentPrefersFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passphrase")
	if err := os.WriteFile(path, []byte("secret-from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLINT_PASSPHRASE", "secret-from-env")
	t.Setenv("FLINT_PASSPHRASE_FILE", path)

	got, configured, err := passphraseFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if !configured || got != "secret-from-file" {
		t.Fatalf("passphrase = %q, configured = %v", got, configured)
	}
}

func TestPassphraseFromEnvironmentRejectsShortValue(t *testing.T) {
	t.Setenv("FLINT_PASSPHRASE", "short")
	if _, _, err := passphraseFromEnvironment(); err == nil {
		t.Fatal("short passphrase was accepted")
	}
}
