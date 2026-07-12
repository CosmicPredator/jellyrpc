package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// AssetResolver turns a plain, publicly-reachable image URL into a Discord
// "mp:external/..." asset key that can be used as an Activity's
// large_image/small_image. Discord requires images to be registered this
// way rather than just accepting arbitrary URLs directly in the activity
// payload — the server has to fetch and proxy the image itself first.
//
// Requires a real Discord application ID (DISCORD_APP_ID) — create one for
// free at https://discord.com/developers/applications, no bot needed.
type AssetResolver struct {
	token string
	appID string
	http  *http.Client

	mu    sync.Mutex
	cache map[string]cachedAsset
}

type cachedAsset struct {
	key       string
	expiresAt time.Time
}

func NewAssetResolver(token, appID string) *AssetResolver {
	return &AssetResolver{
		token: token,
		appID: appID,
		http:  &http.Client{Timeout: 10 * time.Second},
		cache: make(map[string]cachedAsset),
	}
}

// Resolve returns the "mp:..." key for imageURL, using a cached value if
// we've resolved it recently. imageURL must be fetchable by Discord's
// servers (public HTTPS), not a localhost/LAN-only address.
func (r *AssetResolver) Resolve(imageURL string) (string, error) {
	if r.appID == "" {
		return "", fmt.Errorf("DISCORD_APP_ID is not set; can't resolve external image assets")
	}
	if imageURL == "" {
		return "", fmt.Errorf("empty image URL")
	}

	r.mu.Lock()
	if c, ok := r.cache[imageURL]; ok && time.Now().Before(c.expiresAt) {
		r.mu.Unlock()
		return c.key, nil
	}
	r.mu.Unlock()

	body, err := json.Marshal(map[string][]string{"urls": {imageURL}})
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://discord.com/api/v9/applications/%s/external-assets", r.appID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", r.token)

	resp, err := r.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("external-assets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("external-assets returned status %d (check DISCORD_APP_ID and that %s is publicly reachable)", resp.StatusCode, imageURL)
	}

	var results []struct {
		ExternalAssetPath string `json:"external_asset_path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return "", fmt.Errorf("decoding external-assets response: %w", err)
	}
	if len(results) == 0 || results[0].ExternalAssetPath == "" {
		return "", fmt.Errorf("discord returned no asset path for %s", imageURL)
	}

	key := "mp:" + results[0].ExternalAssetPath

	r.mu.Lock()
	r.cache[imageURL] = cachedAsset{key: key, expiresAt: time.Now().Add(3 * time.Hour)}
	r.mu.Unlock()

	return key, nil
}