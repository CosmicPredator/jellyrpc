# jellyrpc

Mirrors your Jellyfin now-playing status onto your Discord Rich Presence,
without needing the Discord desktop client or its local IPC socket. It
talks straight to the Discord Gateway (websocket) and polls Jellyfin's
REST API — so it runs fine on a headless server.

## ⚠️ Read this before using it

Normal Discord Rich Presence works because a game/app talks to the
**Discord desktop client** over a local IPC socket, and the client (which
is logged in as you) relays it to Discord's servers. There is no officially
supported way to drive Rich Presence on a *personal user account* without
that client in the loop.

This tool works around that by connecting to the Gateway directly with your
account's own token, the same way the desktop client would. Automating a
user account this way is generally called a "self-bot," and **it's against
Discord's Terms of Service**, regardless of what it's used for. Discord's
anti-abuse systems can flag this, and consequences run from warnings up to
account suspension — there's no guaranteed-safe way to do this on a
personal account. If that risk isn't one you want to take, the
ToS-compliant alternative is to run this against a **bot account**
instead (Discord Developer Portal → New Application → Bot), which supports
gateway presence updates natively — the tradeoff is the status shows next to
the bot's own name, not yours.

Also treat `DISCORD_TOKEN` like your password: whoever has it can fully
control your account. Never commit it, never share logs containing it.

## How it works

1. Polls `GET {JELLYFIN_URL}/Sessions` every `POLL_INTERVAL_SECONDS` and
   looks for a session belonging to `JELLYFIN_USER_ID` with an active
   `NowPlayingItem`.
2. Converts that into a Discord `Activity` (Listening for music, Watching
   for video, with title/artist/episode info and a progress bar).
3. Maintains a persistent websocket connection to
   `wss://gateway.discord.gg`, identifies with your token, and sends a
   Presence Update (`op 3`) whenever the derived activity changes.
4. Handles heartbeats and reconnects automatically. Clears your presence on
   shutdown or after `IDLE_TIMEOUT_SECONDS` of nothing playing.

## Getting your Discord user token

Open Discord in a browser, open DevTools (F12) → Network tab, reload,
click any request to `discord.com/api`, and find the `authorization`
header in the request headers — that's your token. (Exact steps shift
slightly as Discord tweaks DevTools; search "find discord token network
tab" if the UI has moved.)

## Getting Jellyfin values

- `JELLYFIN_API_KEY`: Jellyfin admin dashboard → Advanced → API Keys →
  create one.
- `JELLYFIN_USER_ID`: Dashboard → Users → click your user → the ID is in
  the page URL (`.../userdetails?userId=<this>`).

## Configuration (environment variables)

| Variable | Required | Default | Notes |
|---|---|---|---|
| `DISCORD_TOKEN` | yes | — | your account token |
| `JELLYFIN_URL` | yes | — | e.g. `https://jellyfin.example.com` — must be **publicly reachable over HTTPS** if you want poster art (see below) |
| `JELLYFIN_API_KEY` | yes | — | |
| `JELLYFIN_USER_ID` | yes | — | only this user's sessions are watched |
| `DISCORD_APP_ID` | no, but required for any image | empty | see "Getting artwork to show up" below |
| `ARTWORK_MODE` | no | `auto` | `auto`/`direct`/`rehost` — see below |
| `DISCORD_SMALL_IMAGE_KEY` | no | empty | optional static badge overlay, see below |
| `POLL_INTERVAL_SECONDS` | no | `15` | how often to poll Jellyfin |
| `IDLE_TIMEOUT_SECONDS` | no | `60` | grace period before clearing presence |

### Getting artwork to show up

Discord silently drops any image on an activity unless `application_id` is
a real, existing Discord application — there is no way around this, even
for a bare text status. So:

1. Create a free application at
   https://discord.com/developers/applications (no bot required — just
   "New Application", give it any name). Copy its **Application ID** into
   `DISCORD_APP_ID`.
2. That's it for dynamic artwork. The daemon automatically pulls the
   actual poster (or series poster, for episodes) / album art from your
   Jellyfin server and registers it with Discord's external-assets API
   to get it rendering as the large image — no manual asset upload
   needed.

**If `JELLYFIN_URL` is a private address** (a `192.168.x.x`/`10.x.x.x`
LAN IP, `localhost`, `.local`/`.lan`, etc — the normal case for a
self-hosted server), Discord's own servers can't reach it directly to
fetch the artwork. In that case the daemon automatically:

1. downloads the poster/cover bytes from Jellyfin itself (using
   `JELLYFIN_API_KEY`, over the private network — this leg doesn't need
   to be public),
2. re-uploads them anonymously to
   [litterbox.catbox.moe](https://litterbox.catbox.moe) (a free temporary
   file host) to get a short-lived public URL,
3. hands *that* URL to Discord's external-assets API as before.

This is on by default (`ARTWORK_MODE=auto`, detected from `JELLYFIN_URL`).
You can force it explicitly with `ARTWORK_MODE=rehost`, or force the
direct behavior with `ARTWORK_MODE=direct` if you do have a public
Jellyfin URL. Two things worth knowing about the rehost path:

- **Litterbox links are anonymous and public** — anyone who gets the link
  (during its ~12h lifetime, refreshed automatically as needed) can view
  that poster/cover image. Fine for movie posters and album art; not
  something you'd want for anything actually sensitive.
- It adds a network round-trip per new item (cached afterwards, so it's
  not hit on every poll — only when what's playing changes).

If artwork still doesn't resolve, check the logs for lines starting with
`artwork:` — they'll say exactly which step failed (fetching from
Jellyfin, uploading to Litterbox, or Discord's resolve call).

`DISCORD_SMALL_IMAGE_KEY` is different: it's a small static badge (e.g. a
Jellyfin logo) shown as a circular overlay on the corner of the poster.
Since it doesn't change per item, it does need to be a manually uploaded
asset: go to your application → Rich Presence → Art Assets, upload an
image, and put its asset key (the name you gave it, lowercased) here.
Leave it unset if you don't want a badge.

## Build & run

```bash
go build -o jellyrpc .

export DISCORD_TOKEN="..."
export JELLYFIN_URL="https://jellyfin.example.com"
export JELLYFIN_API_KEY="..."
export JELLYFIN_USER_ID="..."

./jellyrpc
```

## Running as a systemd service

See `jellyrpc.service`. Put your secrets in `/etc/jellyrpc.env`
(`chmod 600` it) and:

```bash
sudo cp jellyrpc /usr/local/bin/jellyrpc
sudo cp jellyrpc.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now jellyrpc
journalctl -u jellyrpc -f
```