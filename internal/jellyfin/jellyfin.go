package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	userID  string
	http    *http.Client
}

func New(baseURL, apiKey, userID string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		userID:  userID,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// NowPlayingItem is the current playback state for our target user,
// derived from the raw session payload.
type NowPlayingItem struct {
	ID              string // Jellyfin item ID, needed to build an artwork URL
	PrimaryImageTag string // cache-busting tag for the Primary (poster/cover) image, empty if none
	MediaType       string // "Video" or "Audio" (Jellyfin's own MediaType)
	ItemType        string // "Movie", "Episode", "Audio", etc. (Jellyfin's Type)
	Name            string
	SeriesName      string // populated for episodes
	SeriesID        string // populated for episodes; series poster looks better than episode thumb
	SeriesImageTag  string
	Album           string // populated for music
	Artists         []string
	SeasonNumber    int
	EpisodeNum      int
	IsPaused        bool
	PositionTs      time.Duration // current playback position
	RuntimeTs       time.Duration // total runtime, 0 if unknown
}

// session mirrors the subset of Jellyfin's /Sessions response we care about.
type session struct {
	UserID    string `json:"UserId"`
	PlayState struct {
		IsPaused      bool  `json:"IsPaused"`
		PositionTicks int64 `json:"PositionTicks"`
	} `json:"PlayState"`
	NowPlayingItem *struct {
		ID                    string            `json:"Id"`
		Name                  string            `json:"Name"`
		SeriesName            string            `json:"SeriesName"`
		SeriesID              string            `json:"SeriesId"`
		Album                 string            `json:"Album"`
		Artists               []string          `json:"Artists"`
		Type                  string            `json:"Type"`
		MediaType             string            `json:"MediaType"`
		ParentIndexNumber     int               `json:"ParentIndexNumber"`
		IndexNumber           int               `json:"IndexNumber"`
		RunTimeTicks          int64             `json:"RunTimeTicks"`
		ImageTags             map[string]string `json:"ImageTags"`
		SeriesPrimaryImageTag string            `json:"SeriesPrimaryImageTag"`
	} `json:"NowPlayingItem"`
}

// ticksToDuration converts Jellyfin's 100ns "ticks" to time.Duration.
func ticksToDuration(ticks int64) time.Duration {
	return time.Duration(ticks*100) * time.Nanosecond
}

// GetNowPlaying returns the current playback item for the configured user,
// or nil if that user has no active playback session.
func (c *Client) GetNowPlaying() (*NowPlayingItem, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/Sessions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Token", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting jellyfin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jellyfin returned status %d", resp.StatusCode)
	}

	var sessions []session
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("decoding jellyfin response: %w", err)
	}

	for _, s := range sessions {
		if s.UserID != c.userID || s.NowPlayingItem == nil {
			continue
		}
		item := s.NowPlayingItem
		return &NowPlayingItem{
			ID:              item.ID,
			PrimaryImageTag: item.ImageTags["Primary"],
			MediaType:       item.MediaType,
			ItemType:        item.Type,
			Name:            item.Name,
			SeriesName:      item.SeriesName,
			SeriesID:        item.SeriesID,
			SeriesImageTag:  item.SeriesPrimaryImageTag,
			Album:           item.Album,
			Artists:         item.Artists,
			SeasonNumber:    item.ParentIndexNumber,
			EpisodeNum:      item.IndexNumber,
			IsPaused:        s.PlayState.IsPaused,
			PositionTs:      ticksToDuration(s.PlayState.PositionTicks),
			RuntimeTs:       ticksToDuration(item.RunTimeTicks),
		}, nil
	}

	return nil, nil
}

// FetchArtworkBytes downloads the image at artworkURL (as returned by
// ArtworkURL) using our Jellyfin API key, returning the raw bytes and the
// response's Content-Type (e.g. "image/jpeg"). Used when JellyfinURL isn't
// publicly reachable, so we can re-host the bytes somewhere Discord can
// fetch instead of handing Discord the private URL directly.
func (c *Client) FetchArtworkBytes(artworkURL string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, artworkURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Emby-Token", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetching artwork: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("jellyfin returned status %d for artwork", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading artwork body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return data, contentType, nil
}

// ArtworkURL builds a public, directly-fetchable URL for the item's poster
// art (series poster for episodes, since that generally looks better than
// an episode thumbnail; the item's own Primary image otherwise). Returns ""
// if there's no image to show.
//
// Discord's servers fetch this URL directly when resolving it as an
// external Rich Presence asset, so JellyfinURL must be a public HTTPS
// address reachable from the internet — not localhost or a LAN-only IP.
// If your Jellyfin isn't publicly reachable, use FetchArtworkBytes plus a
// re-hosting step (see internal/artwork) instead of handing this URL
// straight to Discord.
func (c *Client) ArtworkURL(item *NowPlayingItem) string {
	if item == nil {
		return ""
	}
	if item.ItemType == "Episode" && item.SeriesID != "" && item.SeriesImageTag != "" {
		return fmt.Sprintf("%s/Items/%s/Images/Primary?tag=%s", c.baseURL, item.SeriesID, item.SeriesImageTag)
	}
	if item.ID != "" && item.PrimaryImageTag != "" {
		return fmt.Sprintf("%s/Items/%s/Images/Primary?tag=%s", c.baseURL, item.ID, item.PrimaryImageTag)
	}
	return ""
}