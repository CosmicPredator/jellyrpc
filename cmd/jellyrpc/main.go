// Command jellyrpc watches a Jellyfin user's playback sessions and mirrors
// them onto a Discord account's Rich Presence, entirely over the network
// (Discord Gateway websocket + Jellyfin REST API) — no local Discord
// client, no unix socket, so it works fine on a headless server.
//
// See README.md for setup and, importantly, the ToS caveats of driving
// presence with a personal account token instead of a bot token.
package main

import (
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	"cosmic/jellyrpc/internal/artwork"
	"cosmic/jellyrpc/internal/config"
	"cosmic/jellyrpc/internal/discord"
	"cosmic/jellyrpc/internal/jellyfin"
	"cosmic/jellyrpc/internal/logging"
	"cosmic/jellyrpc/internal/presence"
)

func main() {
	logging.Setup()
	logging.Info("starting jellyrpc")

	cfg, err := config.Load()
	if err != nil {
		logging.Fatal("config: %v", err)
	}
	logging.Info("loaded config: jellyfin=%s user=%s poll=%s idle=%s artwork=%s",
		cfg.JellyfinURL, cfg.JellyfinUserID, cfg.PollInterval, cfg.IdleTimeout, cfg.ArtworkMode)

	jf := jellyfin.New(cfg.JellyfinURL, cfg.JellyfinAPIKey, cfg.JellyfinUserID)
	gw := discord.New(cfg.DiscordToken)
	assetResolver := discord.NewAssetResolver(cfg.DiscordToken, cfg.DiscordAppID)
	artManager := artwork.NewManager(cfg, jf, assetResolver)

	stop := make(chan struct{})
	logging.Info("starting discord gateway")
	go gw.Run(stop)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	var lastActivity *discord.Activity
	var lastSeenPlaying time.Time

	poll := func() {
		item, err := jf.GetNowPlaying()
		if err != nil {
			logging.Warn("jellyfin poll failed: %v", err)
			return
		}

		if item == nil {
			// Nothing playing right now. Don't clear immediately —
			// give it IdleTimeout in case this is just a brief pause
			// between episodes / a Jellyfin hiccup.
			if !lastSeenPlaying.IsZero() && time.Since(lastSeenPlaying) > cfg.IdleTimeout {
				if lastActivity != nil {
					logging.Info("nothing playing, clearing presence")
					if err := gw.SetActivity(nil); err != nil {
						logging.Warn("clearing presence: %v", err)
					} else {
						lastActivity = nil
					}
				}
			}
			return
		}

		lastSeenPlaying = time.Now()

		assetKey := ""
		if cfg.DiscordAppID != "" {
			key, err := artManager.Resolve(item)
			if err != nil {
				logging.Warn("artwork: %v (continuing without image)", err)
			} else {
				assetKey = key
			}
		}
		act := presence.Build(cfg, item, assetKey)

		if !activitiesEqual(lastActivity, act) {
			if err := gw.SetActivity(act); err != nil {
				logging.Warn("updating presence: %v", err)
				return
			}
			lastActivity = act
			logging.Success("presence updated: %s / %s", act.Details, act.State)
		}
	}

	logging.Info("performing initial jellyfin poll")
	poll()

	for {
		select {
		case <-ticker.C:
			poll()
		case <-sig:
			logging.Info("shutting down, clearing presence")
			_ = gw.SetActivity(nil)
			close(stop)
			time.Sleep(200 * time.Millisecond) // let the clear frame flush
			return
		}
	}
}

func activitiesEqual(a, b *discord.Activity) bool {
	if a == nil || b == nil {
		return a == b
	}
	// Ignore timestamps in the comparison so we don't spam presence
	// updates every poll just because a couple of seconds elapsed.
	ac, bc := *a, *b
	ac.Timestamps, bc.Timestamps = nil, nil
	return reflect.DeepEqual(ac, bc)
}
