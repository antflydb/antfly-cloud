package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigLoadSaveAndEnvPrecedence(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(work)
	cfg := Config{APIURL: "https://api.example/v1", Token: "file-token", Org: "file-org", Instance: "file-instance"}
	if err := saveConfig("", cfg); err != nil {
		t.Fatal(err)
	}
	path, err := configPath("")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != defaultConfigFile {
		t.Fatalf("unexpected path: %s", path)
	}
	if path != filepath.Join(home, configDirName, defaultConfigFile) {
		t.Fatalf("unexpected default path: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config perms = %v, want 0600", got)
	}
	loaded, err := loadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != "file-token" || loaded.Org != "file-org" || loaded.Instance != "file-instance" || loaded.APIURL != "https://api.example/v1" {
		t.Fatalf("loaded %#v", loaded)
	}

	t.Setenv("ANTFLY_CLOUD_TOKEN", "env-token")
	t.Setenv("ANTFLY_CLOUD_ORG", "env-org")
	t.Setenv("ANTFLY_CLOUD_INSTANCE", "env-instance")
	t.Setenv("ANTFLY_CLOUD_API_URL", "https://env.example/v1")
	loaded = applyEnv(loaded)
	if loaded.Token != "env-token" || loaded.Org != "env-org" || loaded.Instance != "env-instance" || loaded.APIURL != "https://env.example/v1" {
		t.Fatalf("env loaded %#v", loaded)
	}
}

func TestConfigDiscoveryPrefersLocalDirectory(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(work)

	if err := saveConfig(filepath.Join(home, configDirName), Config{APIURL: "https://home.example/api/v1", Token: "home"}); err != nil {
		t.Fatal(err)
	}
	localDir := filepath.Join(work, configDirName)
	if err := saveConfig(localDir, Config{APIURL: "https://local.example/api/v1", Token: "local"}); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != "local" || loaded.APIURL != "https://local.example/api/v1" {
		t.Fatalf("loaded %#v", loaded)
	}
	path, err := configPath("")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(localDir, defaultConfigFile) {
		t.Fatalf("configPath = %s, want local config", path)
	}
}

func TestConfigSaveJSONWhenPathUsesJSONExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := saveConfig(path, Config{
		APIURL: "https://json.example/api/v1",
		Auth: AuthConfig{
			Type:         authTypeOIDC,
			IDToken:      "json-token",
			RefreshToken: "json-refresh",
			ExpiresAt:    expiresAt,
		},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[0] != '{' {
		t.Fatalf("expected JSON config, got %q", string(data))
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.IDToken != "json-token" || loaded.Auth.RefreshToken != "json-refresh" || !loaded.Auth.ExpiresAt.Equal(expiresAt) || loaded.APIURL != "https://json.example/api/v1" {
		t.Fatalf("loaded %#v", loaded)
	}
}

func TestConfigBearerTokenPrecedence(t *testing.T) {
	if got := (Config{Token: "stored-token", Auth: AuthConfig{Type: authTypeOIDC, IDToken: "oidc"}}).bearerToken(); got != "stored-token" {
		t.Fatalf("bearerToken = %q, want stored-token", got)
	}
	if got := (Config{Auth: AuthConfig{Type: authTypeOIDC, IDToken: "oidc"}}).bearerToken(); got != "oidc" {
		t.Fatalf("bearerToken = %q, want oidc", got)
	}
	if got := (Config{Auth: AuthConfig{Type: authTypePAT, Token: "pat"}}).bearerToken(); got != "pat" {
		t.Fatalf("bearerToken = %q, want pat", got)
	}
}
