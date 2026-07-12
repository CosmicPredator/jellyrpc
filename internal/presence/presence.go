package presence

import (
	"fmt"
	"strings"
	"time"

	"cosmic/jellyrpc/internal/config"
	"cosmic/jellyrpc/internal/discord"
	"cosmic/jellyrpc/internal/jellyfin"
)

const (
	activityTypeListening = 2
	activityTypeWatching  = 3
)

// Build converts a Jellyfin now-playing item into a Discord Activity, or
// returns nil if there's nothing to show.
//
// assetKey, if non-empty, should already be a resolved Discord "mp:" asset
// key (see internal/artwork) for the item's poster/album art. Pass "" to
// omit an image. cfg.DiscordAppID is required for any image to render at
// all, so we only attach assets when it's set.
func Build(cfg *config.Config, item *jellyfin.NowPlayingItem, assetKey string) *discord.Activity {
	if item == nil {
		return nil
	}

	act := &discord.Activity{
		ApplicationID: cfg.DiscordAppID,
	}

	switch item.MediaType {
	case "Audio":
		act.Type = activityTypeListening
		if len(item.Artists) > 0 {
			act.Name = strings.Join(item.Artists, ", ")
		} else {
			act.Name = "Jellyfin"
		}
		act.Details = item.Name
		if len(item.Artists) > 0 {
			act.State = strings.Join(item.Artists, ", ")
		}
	default: // Video: Movie, Episode, etc.
		act.Type = activityTypeWatching
		act.Name = "Jellyfin"
		switch item.ItemType {
		case "Episode":
			act.Details = item.SeriesName
			if item.SeasonNumber > 0 && item.EpisodeNum > 0 {
				act.State = fmt.Sprintf("S%02dE%02d · %s", item.SeasonNumber, item.EpisodeNum, item.Name)
			} else {
				act.State = item.Name
			}
		default: // Movie and anything else
			act.Details = item.Name
		}
	}

	if item.IsPaused {
		act.State = strings.TrimSpace(act.State + " (paused)")
	} else if item.RuntimeTs > 0 {
		now := time.Now()
		start := now.Add(-item.PositionTs)
		end := start.Add(item.RuntimeTs)
		act.Timestamps = &discord.ActivityTimestamps{
			Start: start.UnixMilli(),
			End:   end.UnixMilli(),
		}
	}

	if cfg.DiscordAppID != "" || assetKey != "" {
		act.Assets = &discord.ActivityAssets{}

		if assetKey != "" {
			act.Assets.LargeImage = assetKey
			act.Assets.LargeText = act.Details
		}
	}

	return act
}
