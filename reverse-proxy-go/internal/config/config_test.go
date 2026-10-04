package config

import (
	"os"
	"testing"
)

// TestLoadValidConfig tests loading a valid configuration.
func TestLoadValidConfig(t *testing.T) {
	configData := `{
		"listenAddr": ":9090",
		"routes": [
			{
				"match": { "pathPrefix": "/api" },
				"upstream": "http://localhost:3000",
				"stripPrefix": true
			}
		]
	}`

	tmpFile := createTempConfigFile(t, configData)
	defer os.Remove(tmpFile)

	config, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.ListenAddr != ":9090" {
		t.Fatalf("expected listenAddr ':9090', got %q", config.ListenAddr)
	}

	if len(config.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(config.Routes))
	}

	if config.Routes[0].Upstream != "http://localhost:3000" {
		t.Fatalf("expected upstream 'http://localhost:3000'")
	}
}

// TestLoadConfigMissingFile tests error when config file doesn't exist.
func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.json")
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

// TestLoadConfigInvalidJSON tests error on invalid JSON.
func TestLoadConfigInvalidJSON(t *testing.T) {
	tmpFile := createTempConfigFile(t, `{ invalid json }`)
	defer os.Remove(tmpFile)

	_, err := LoadConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

// TestLoadConfigMissingUpstream tests validation error.
func TestLoadConfigMissingUpstream(t *testing.T) {
	configData := `{
		"listenAddr": ":8080",
		"routes": [
			{
				"match": { "pathPrefix": "/api" }
			}
		]
	}`

	tmpFile := createTempConfigFile(t, configData)
	defer os.Remove(tmpFile)

	_, err := LoadConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected validation error for missing upstream")
	}
}

// TestLoadConfigMissingMatch tests validation error.
func TestLoadConfigMissingMatch(t *testing.T) {
	configData := `{
		"listenAddr": ":8080",
		"routes": [
			{
				"upstream": "http://localhost:3000"
			}
		]
	}`

	tmpFile := createTempConfigFile(t, configData)
	defer os.Remove(tmpFile)

	_, err := LoadConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected validation error for missing match criteria")
	}
}

// TestLoadConfigDefaultListenAddr tests default listen address.
func TestLoadConfigDefaultListenAddr(t *testing.T) {
	configData := `{
		"routes": [
			{
				"match": { "pathPrefix": "/api" },
				"upstream": "http://localhost:3000"
			}
		]
	}`

	tmpFile := createTempConfigFile(t, configData)
	defer os.Remove(tmpFile)

	config, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.ListenAddr != ":8080" {
		t.Fatalf("expected default listenAddr ':8080', got %q", config.ListenAddr)
	}
}

// TestLoadConfigNoRoutes tests error when no routes configured.
func TestLoadConfigNoRoutes(t *testing.T) {
	configData := `{
		"listenAddr": ":8080",
		"routes": []
	}`

	tmpFile := createTempConfigFile(t, configData)
	defer os.Remove(tmpFile)

	_, err := LoadConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected error for empty routes")
	}
}

// TestDefaultConfig tests the default configuration.
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.ListenAddr != ":8080" {
		t.Fatalf("expected default listenAddr ':8080'")
	}

	if len(config.Routes) != 1 {
		t.Fatalf("expected 1 default route")
	}
}

// createTempConfigFile creates a temporary config file for testing.
func createTempConfigFile(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "config*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	return tmpFile.Name()
}
