package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds everything the daemon needs. Everything is supplied via
// environment variables so the binary works well as a systemd service /
// docker container.
type Config struct {
	// DiscordToken is your Discord *account* token (NOT a bot token).
	// Treat it exactly like a password: anyone with it has full control
	// of your Discord account.
	DiscordToken string

	// DiscordAppID is required for ANY image to show up on the presence
	// card (Discord silently drops image keys otherwise). Create a free
	// application at https://discord.com/developers/applications (no bot
	// needed) and put its Application ID here. With this set, the
	// daemon automatically shows the actual Jellyfin poster/album art by
	// resolving it through Discord's external-assets API. Leave empty
	// for a plain text-only presence.
	DiscordAppID string

	JellyfinURL    string // e.g. https://jellyfin.example.com
	JellyfinAPIKey string // Jellyfin admin dashboard -> API keys
	JellyfinUserID string // Jellyfin dashboard -> Users -> (user) -> copy ID from URL

	PollInterval time.Duration
	// IdleTimeout: if Jellyfin reports nothing playing for longer than
	// this, we clear the Discord presence instead of leaving stale info.
	IdleTimeout time.Duration

	// SmallImageURL is an optional STATIC image URL (e.g. a small
	// Jellyfin logo badge) shown as a small overlay on the large,
	// dynamically-resolved poster/album art. Leave empty to omit it.
	SmallImageURL string

	// ArtworkMode controls how poster/album art reaches Discord:
	//   "auto"    - rehost via Litterbox if JellyfinURL looks private
	//               (LAN IP, localhost, .local/.lan/etc), otherwise hand
	//               Discord the JellyfinURL directly. Default.
	//   "direct"  - always hand Discord the JellyfinURL directly. Only
	//               works if it's actually publicly reachable over HTTPS.
	//   "rehost"  - always download the image and re-upload it to
	//               litterbox.catbox.moe first, then give Discord that
	//               temporary public URL. Use this if JellyfinURL is
	//               private (e.g. 192.168.x.x) so Discord's servers can
	//               still fetch the image. Note: uploads are anonymous
	//               and publicly viewable for their lifetime.
	ArtworkMode string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() (*Config, error) {
	cfg := &Config{
		DiscordToken:   os.Getenv("DISCORD_TOKEN"),
		DiscordAppID:   os.Getenv("DISCORD_APP_ID"),
		JellyfinURL:    os.Getenv("JELLYFIN_URL"),
		JellyfinAPIKey: os.Getenv("JELLYFIN_API_KEY"),
		JellyfinUserID: os.Getenv("JELLYFIN_USER_ID"),
		ArtworkMode:    getenv("ARTWORK_MODE", "auto"),
	}

	pollSeconds, err := strconv.Atoi(getenv("POLL_INTERVAL_SECONDS", getenv("POLL_SECS", "15")))
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL_SECONDS/POLL_SECS: %w", err)
	}
	cfg.PollInterval = time.Duration(pollSeconds) * time.Second

	idleSeconds, err := strconv.Atoi(getenv("IDLE_TIMEOUT_SECONDS", getenv("TIMEOUT_SECS", "60")))
	if err != nil {
		return nil, fmt.Errorf("invalid IDLE_TIMEOUT_SECONDS/TIMEOUT_SECS: %w", err)
	}
	cfg.IdleTimeout = time.Duration(idleSeconds) * time.Second

	if cfg.DiscordToken == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN is required")
	}
	if cfg.JellyfinURL == "" {
		return nil, fmt.Errorf("JELLYFIN_URL is required")
	}
	if cfg.JellyfinAPIKey == "" {
		return nil, fmt.Errorf("JELLYFIN_API_KEY is required")
	}
	if cfg.JellyfinUserID == "" {
		return nil, fmt.Errorf("JELLYFIN_USER_ID is required")
	}

	return cfg, nil
}
