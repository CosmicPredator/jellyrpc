package config

import (
	"testing"
	"time"
)

func TestLoadReadsShortEnvAliasesForIntervals(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "token")
	t.Setenv("JELLYFIN_URL", "https://jellyfin.example")
	t.Setenv("JELLYFIN_API_KEY", "api-key")
	t.Setenv("JELLYFIN_USER_ID", "user-id")
	t.Setenv("POLL_INTERVAL_SECONDS", "")
	t.Setenv("IDLE_TIMEOUT_SECONDS", "")
	t.Setenv("POLL_SECS", "7")
	t.Setenv("TIMEOUT_SECS", "11")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.PollInterval != 7*time.Second {
		t.Fatalf("expected poll interval 7s, got %s", cfg.PollInterval)
	}
	if cfg.IdleTimeout != 11*time.Second {
		t.Fatalf("expected idle timeout 11s, got %s", cfg.IdleTimeout)
	}
}
