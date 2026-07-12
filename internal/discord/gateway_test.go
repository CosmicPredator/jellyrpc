package discord

import (
	"net"
	"testing"
)

func TestBuildPresenceUpdateUsesInvisibleStatusWhenClearing(t *testing.T) {
	update := buildPresenceUpdate(nil, "invisible")

	if update.Status != "invisible" {
		t.Fatalf("expected invisible status, got %q", update.Status)
	}
	if len(update.Activities) != 0 {
		t.Fatalf("expected no activities when clearing, got %d", len(update.Activities))
	}
}

func TestBuildPresenceUpdateUsesOnlineStatusWhenActivityPresent(t *testing.T) {
	activity := Activity{Name: "Jellyfin", Type: 2}
	update := buildPresenceUpdate(&activity, "online")

	if update.Status != "online" {
		t.Fatalf("expected online status, got %q", update.Status)
	}
	if len(update.Activities) != 1 {
		t.Fatalf("expected one activity, got %d", len(update.Activities))
	}
	if update.Activities[0].Name != activity.Name {
		t.Fatalf("expected activity name %q, got %q", activity.Name, update.Activities[0].Name)
	}
}

func TestSetActivityRequestsReconnectWhenDisconnectedAndPresenceReturns(t *testing.T) {
	g := &Gateway{reconnectCh: make(chan struct{}, 1)}
	activity := &Activity{Name: "Jellyfin", Type: 2}

	if err := g.SetActivity(activity); err != nil {
		t.Fatalf("SetActivity returned error: %v", err)
	}

	select {
	case <-g.reconnectCh:
	default:
		t.Fatal("expected reconnect request when activity is set while disconnected")
	}
}

func TestDisconnectCanBeCalledMoreThanOnce(t *testing.T) {
	g := &Gateway{stopHeartbeat: make(chan struct{})}

	g.disconnect()
	g.disconnect()
}

func TestExpectedDisconnectErrorIsRecognized(t *testing.T) {
	if !isExpectedDisconnectError(net.ErrClosed) {
		t.Fatal("expected net.ErrClosed to be treated as a disconnect")
	}
}
