package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContainerConfigDerivesPrivateServiceEnvironment(t *testing.T) {
	stateRoot := t.TempDir()
	repositoryRoot := filepath.Join(t.TempDir(), "repos")
	t.Setenv("CODEFLOW_DOMAIN", "codeflow.example.test")
	t.Setenv("POCKET_ID_DOMAIN", "auth.example.test")
	t.Setenv("POCKET_ID_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("CODEFLOW_STATE_ROOT", stateRoot)
	t.Setenv("CODEFLOW_REPOSITORIES_PATH", repositoryRoot)
	for _, key := range []string{"CODEFLOW_MCP_RESOURCE", "CODEFLOW_MCP_OAUTH_ISSUER", "CODEFLOW_MCP_OAUTH_JWKS_URL", "CODEFLOW_DATA", "APP_URL", "DB_CONNECTION_STRING", "ENCRYPTION_KEY"} {
		t.Setenv(key, "")
	}

	config, err := loadContainerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.prepare(); err != nil {
		t.Fatal(err)
	}
	if err := config.applyEnvironment(); err != nil {
		t.Fatal(err)
	}
	assertEnvironment(t, "CODEFLOW_MCP_RESOURCE", "https://codeflow.example.test")
	assertEnvironment(t, "CODEFLOW_MCP_OAUTH_ISSUER", "https://auth.example.test")
	assertEnvironment(t, "CODEFLOW_MCP_OAUTH_JWKS_URL", "http://127.0.0.1:1411/.well-known/jwks.json")
	assertEnvironment(t, "APP_URL", "https://auth.example.test")
	assertEnvironment(t, "CODEFLOW_DISCOVERY_ROOTS", repositoryRoot)
	assertEnvironment(t, "ALLOW_INSECURE_CALLBACK_URLS", "false")
	if _, err := os.Stat(filepath.Join(stateRoot, "pocket-id")); err != nil {
		t.Fatal(err)
	}
}

func TestContainerConfigRejectsUnsafeOrIncompleteSetup(t *testing.T) {
	tests := []struct {
		name       string
		codeflow   string
		pocket     string
		encryption string
	}{
		{name: "missing domain", pocket: "auth.example.test", encryption: "0123456789abcdef"},
		{name: "scheme in domain", codeflow: "https://codeflow.example.test", pocket: "auth.example.test", encryption: "0123456789abcdef"},
		{name: "shared domain", codeflow: "same.example.test", pocket: "same.example.test", encryption: "0123456789abcdef"},
		{name: "short encryption key", codeflow: "codeflow.example.test", pocket: "auth.example.test", encryption: "short"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CODEFLOW_DOMAIN", test.codeflow)
			t.Setenv("POCKET_ID_DOMAIN", test.pocket)
			t.Setenv("POCKET_ID_ENCRYPTION_KEY", test.encryption)
			t.Setenv("POCKET_ID_ENCRYPTION_KEY_FILE", "")
			if _, err := loadContainerConfig(); err == nil {
				t.Fatal("configuration unexpectedly succeeded")
			}
		})
	}
}

func assertEnvironment(t *testing.T, key, want string) {
	t.Helper()
	if got := os.Getenv(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
