package discord

import "encoding/json"

// Gateway opcodes we care about (see discord.com/developers/docs/topics/opcodes-and-status-codes).
const (
	opDispatch       = 0
	opHeartbeat      = 1
	opIdentify       = 2
	opPresenceUpdate = 3
	opReconnect      = 7
	opInvalidSession = 9
	opHello          = 10
	opHeartbeatACK   = 11
)

type payload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int            `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

type helloData struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

type identifyProperties struct {
	OS      string `json:"os"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
}

type identifyData struct {
	Token      string             `json:"token"`
	Properties identifyProperties `json:"properties"`
	Presence   *presenceUpdate    `json:"presence,omitempty"`
	// Compress/large_threshold intentionally omitted; not needed for a
	// presence-only client and keeps the payload minimal.
}

// ActivityTimestamps lets Discord show an elapsed ("00:03 elapsed") or
// remaining counter next to the activity.
type ActivityTimestamps struct {
	Start int64 `json:"start,omitempty"` // unix ms
	End   int64 `json:"end,omitempty"`   // unix ms
}

type ActivityAssets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

// Activity type values:
// 0 Playing, 1 Streaming, 2 Listening, 3 Watching, 4 Custom, 5 Competing
type Activity struct {
	Name          string              `json:"name"`
	Type          int                 `json:"type"`
	Details       string              `json:"details,omitempty"`
	State         string              `json:"state,omitempty"`
	Timestamps    *ActivityTimestamps `json:"timestamps,omitempty"`
	Assets        *ActivityAssets     `json:"assets,omitempty"`
	ApplicationID string              `json:"application_id,omitempty"`
}

type presenceUpdate struct {
	Since      *int64     `json:"since"`
	Activities []Activity `json:"activities"`
	Status     string     `json:"status"`
	AFK        bool       `json:"afk"`
}
