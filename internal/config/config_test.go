package config

import (
	"strings"
	"testing"
)

func TestLoadAcceptsMireyeAlias(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/db")
	t.Setenv("MIREYE_API_TOKEN", "")
	t.Setenv("MIREYE_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MireyeToken != "test-token" {
		t.Fatalf("MireyeToken = %q, want alias value", cfg.MireyeToken)
	}
}

func TestLoadNamesMissingVariableWithoutValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("MIREYE_API_TOKEN", "secret-that-must-not-appear")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load() error = %v, want named DATABASE_URL error", err)
	}
	if strings.Contains(err.Error(), "secret-that-must-not-appear") {
		t.Fatal("Load() error leaked a secret")
	}
}

func TestLoadUsesRenderPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/db")
	t.Setenv("MIREYE_API_TOKEN", "test-token")
	t.Setenv("HTTP_ADDRESS", "")
	t.Setenv("PORT", "10000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddress != ":10000" {
		t.Fatalf("HTTPAddress = %q, want :10000", cfg.HTTPAddress)
	}
}
