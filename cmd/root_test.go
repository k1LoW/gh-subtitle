package cmd

import (
	"testing"
)

func TestBuildProviderConfig(t *testing.T) {
	t.Run("copilot without byok returns nil", func(t *testing.T) {
		cfg, err := buildProviderConfig("copilot", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg != nil {
			t.Fatalf("expected nil, got %v", cfg)
		}
	})

	t.Run("non-copilot without byok returns error", func(t *testing.T) {
		_, err := buildProviderConfig("openai", false)
		if err == nil {
			t.Fatal("expected error")
		}
		if got := err.Error(); got != `provider "openai" requires --byok flag` {
			t.Fatalf("unexpected error: %s", got)
		}
	})

	t.Run("copilot with byok returns error", func(t *testing.T) {
		_, err := buildProviderConfig("copilot", true)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("openai with byok and API key", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "sk-test-key")
		// Clear base URL overrides
		baseURL = ""
		t.Setenv("GH_SUBTITLE_PROVIDER_BASE_URL", "")

		cfg, err := buildProviderConfig("openai", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Type != "openai" {
			t.Errorf("expected type openai, got %s", cfg.Type)
		}
		if cfg.BaseURL != "https://api.openai.com/v1" {
			t.Errorf("expected default base URL, got %s", cfg.BaseURL)
		}
		if cfg.APIKey != "sk-test-key" {
			t.Errorf("expected API key sk-test-key, got %s", cfg.APIKey)
		}
	})

	t.Run("openai with byok but no API key", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "")
		baseURL = ""

		_, err := buildProviderConfig("openai", true)
		if err == nil {
			t.Fatal("expected error for missing API key")
		}
	})

	t.Run("anthropic with byok and API key", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "sk-ant-test")
		baseURL = ""
		t.Setenv("GH_SUBTITLE_PROVIDER_BASE_URL", "")

		cfg, err := buildProviderConfig("anthropic", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Type != "anthropic" {
			t.Errorf("expected type anthropic, got %s", cfg.Type)
		}
		if cfg.BaseURL != "https://api.anthropic.com" {
			t.Errorf("expected default base URL, got %s", cfg.BaseURL)
		}
	})

	t.Run("azure with byok requires base URL", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "azure-key")
		baseURL = ""
		t.Setenv("GH_SUBTITLE_PROVIDER_BASE_URL", "")

		_, err := buildProviderConfig("azure", true)
		if err == nil {
			t.Fatal("expected error for missing base URL")
		}
	})

	t.Run("azure with byok and base URL flag", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "azure-key")
		baseURL = "https://myinstance.openai.azure.com"
		t.Setenv("GH_SUBTITLE_PROVIDER_BASE_URL", "")

		cfg, err := buildProviderConfig("azure", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Type != "azure" {
			t.Errorf("expected type azure, got %s", cfg.Type)
		}
		if cfg.BaseURL != "https://myinstance.openai.azure.com" {
			t.Errorf("expected base URL from flag, got %s", cfg.BaseURL)
		}
		baseURL = "" // reset
	})

	t.Run("ollama with byok does not require API key", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "")
		baseURL = ""
		t.Setenv("GH_SUBTITLE_PROVIDER_BASE_URL", "")

		cfg, err := buildProviderConfig("ollama", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Type != "openai" {
			t.Errorf("expected type openai (ollama uses openai-compatible API), got %s", cfg.Type)
		}
		if cfg.BaseURL != "http://localhost:11434/v1" {
			t.Errorf("expected default ollama URL, got %s", cfg.BaseURL)
		}
	})

	t.Run("unsupported provider with byok", func(t *testing.T) {
		_, err := buildProviderConfig("unknown", true)
		if err == nil {
			t.Fatal("expected error for unsupported provider")
		}
	})

	t.Run("base URL from env var", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "sk-test")
		baseURL = ""
		t.Setenv("GH_SUBTITLE_PROVIDER_BASE_URL", "https://custom.example.com/v1")

		cfg, err := buildProviderConfig("openai", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("expected base URL from env, got %s", cfg.BaseURL)
		}
	})

	t.Run("base URL flag takes precedence over env", func(t *testing.T) {
		t.Setenv("GH_SUBTITLE_PROVIDER_API_KEY", "sk-test")
		baseURL = "https://flag.example.com/v1"
		t.Setenv("GH_SUBTITLE_PROVIDER_BASE_URL", "https://env.example.com/v1")

		cfg, err := buildProviderConfig("openai", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != "https://flag.example.com/v1" {
			t.Errorf("expected base URL from flag, got %s", cfg.BaseURL)
		}
		baseURL = "" // reset
	})
}
