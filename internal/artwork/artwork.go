// Package artwork decides how to turn a Jellyfin now-playing item's poster
// art into something Discord will actually render, and caches the result
// so we're not re-uploading/re-resolving on every poll.
package artwork

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"cosmic/jellyrpc/internal/config"
	"cosmic/jellyrpc/internal/discord"
	"cosmic/jellyrpc/internal/jellyfin"
	"cosmic/jellyrpc/internal/litterbox"
	"cosmic/jellyrpc/internal/logging"
)

type Manager struct {
	jf       *jellyfin.Client
	resolver *discord.AssetResolver
	rehost   bool

	mu    sync.Mutex
	cache map[string]cacheEntry // keyed by the (private) Jellyfin artwork URL
}

type cacheEntry struct {
	key       string
	expiresAt time.Time
}

func NewManager(cfg *config.Config, jf *jellyfin.Client, resolver *discord.AssetResolver) *Manager {
	return &Manager{
		jf:       jf,
		resolver: resolver,
		rehost:   shouldRehost(cfg),
		cache:    make(map[string]cacheEntry),
	}
}

// shouldRehost decides, based on ArtworkMode and (in "auto" mode) whether
// JellyfinURL's host looks like a private/local address, whether we need
// to bounce the image through Litterbox before Discord can see it.
func shouldRehost(cfg *config.Config) bool {
	switch strings.ToLower(cfg.ArtworkMode) {
	case "rehost":
		return true
	case "direct":
		return false
	}

	u, err := url.Parse(cfg.JellyfinURL)
	if err != nil {
		return false
	}
	host := u.Hostname()

	if ip := net.ParseIP(host); ip != nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}

	lower := strings.ToLower(host)
	if lower == "localhost" {
		return true
	}
	for _, suffix := range []string{".local", ".lan", ".home", ".internal", ".corp"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// Resolve returns a Discord "mp:" asset key for item's poster art, or ""
// if there's no artwork to show. Results are cached per Jellyfin item
// (keyed by its image URL, which changes when the image itself does) so
// repeated polls of the same now-playing item don't re-upload or re-hit
// Discord's API every time.
func (m *Manager) Resolve(item *jellyfin.NowPlayingItem) (string, error) {
	artworkURL := m.jf.ArtworkURL(item)
	if artworkURL == "" {
		return "", nil
	}

	m.mu.Lock()
	if c, ok := m.cache[artworkURL]; ok && time.Now().Before(c.expiresAt) {
		m.mu.Unlock()
		return c.key, nil
	}
	m.mu.Unlock()

	logging.Info("resolving artwork for %s", item.Name)
	publicURL := artworkURL
	cacheTTL := 3 * time.Hour

	if m.rehost {
		logging.Info("rehosting artwork through litterbox")
		data, contentType, err := m.jf.FetchArtworkBytes(artworkURL)
		if err != nil {
			return "", fmt.Errorf("fetching artwork from jellyfin: %w", err)
		}

		ttl := litterbox.Hour12
		hostedURL, err := litterbox.Upload(data, "art"+extensionFor(contentType), ttl)
		if err != nil {
			return "", fmt.Errorf("uploading artwork to litterbox: %w", err)
		}
		logging.Info("artwork uploaded to litterbox: %s", hostedURL)
		publicURL = hostedURL

		// Renew a bit before the litterbox link actually expires.
		cacheTTL = ttl.Duration() - 30*time.Minute
		if cacheTTL < time.Minute {
			cacheTTL = time.Minute
		}
	}

	key, err := m.resolver.Resolve(publicURL)
	if err != nil {
		return "", fmt.Errorf("resolving discord asset: %w", err)
	}
	logging.Info("discord asset resolved for artwork url %s", publicURL)

	m.mu.Lock()
	m.cache[artworkURL] = cacheEntry{key: key, expiresAt: time.Now().Add(cacheTTL)}
	m.mu.Unlock()

	return key, nil
}

func extensionFor(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".jpg"
	}
}
