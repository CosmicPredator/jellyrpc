// Package discord implements just enough of the Discord Gateway protocol to
// authenticate as a user account and push Rich Presence (activity) updates,
// without needing the official Discord client or its local IPC socket.
//
// IMPORTANT: connecting to the gateway with a personal *user* token (rather
// than a bot token created in the Developer Portal) to automate an account
// is commonly called a "self-bot" and is against Discord's Terms of
// Service. Discord can and does detect and act on this, up to and
// including disabling the account. This code is provided for personal,
// educational use against your own account with that risk understood and
// accepted; there is no way to make this fully "compliant" while keeping
// the "no official client running" requirement, because Rich Presence for
// user accounts is only officially supported through the client's local
// RPC mechanism.
package discord

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"cosmic/jellyrpc/internal/logging"

	"github.com/gorilla/websocket"
)

const gatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"

type Gateway struct {
	token string

	mu            sync.Mutex
	conn          *websocket.Conn
	seq           *int
	sessionID     string
	stopHeartbeat chan struct{}

	// lastActivity is resent after every reconnect so presence survives
	// gateway hiccups.
	lastActivity *Activity
	reconnectCh  chan struct{}
	closing      bool
}

func New(token string) *Gateway {
	return &Gateway{token: token, reconnectCh: make(chan struct{}, 1)}
}

// Run waits for reconnect requests and connects only when activity is
// detected again. It blocks, so call it in a goroutine.
func (g *Gateway) Run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			g.disconnect()
			return
		case <-g.reconnectCh:
			if err := g.connectOnce(stop); err != nil {
				logging.Warn("discord gateway: %v", err)
			}
		}
	}
}

func (g *Gateway) connectOnce(stop <-chan struct{}) error {
	g.mu.Lock()
	g.closing = false
	g.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(gatewayURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	g.mu.Lock()
	g.conn = conn
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		if g.conn == conn {
			g.conn = nil
		}
		g.mu.Unlock()
	}()

	// First message must be Hello.
	var hello payload
	if err := conn.ReadJSON(&hello); err != nil {
		return fmt.Errorf("reading hello: %w", err)
	}
	if hello.Op != opHello {
		return fmt.Errorf("expected hello, got op %d", hello.Op)
	}
	var hd helloData
	if err := json.Unmarshal(hello.D, &hd); err != nil {
		return fmt.Errorf("parsing hello: %w", err)
	}

	heartbeatStop := make(chan struct{})
	g.mu.Lock()
	g.stopHeartbeat = heartbeatStop
	g.mu.Unlock()
	go g.heartbeatLoop(conn, time.Duration(hd.HeartbeatInterval)*time.Millisecond)
	defer func() {
		g.mu.Lock()
		if g.stopHeartbeat == heartbeatStop {
			g.stopHeartbeat = nil
		}
		g.mu.Unlock()
		select {
		case <-heartbeatStop:
		default:
			close(heartbeatStop)
		}
	}()

	if err := g.identify(conn); err != nil {
		return fmt.Errorf("identify: %w", err)
	}

	// Re-send whatever presence we last wanted, in case the reconnect
	// dropped it.
	g.mu.Lock()
	last := g.lastActivity
	g.mu.Unlock()
	if last != nil {
		_ = g.sendPresence(conn, *last)
	}

	for {
		select {
		case <-stop:
			return nil
		case <-heartbeatStop:
			return nil
		default:
		}

		var p payload
		if err := conn.ReadJSON(&p); err != nil {
			if g.isClosing() || isExpectedDisconnectError(err) {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		if p.S != nil {
			g.mu.Lock()
			g.seq = p.S
			g.mu.Unlock()
		}

		switch p.Op {
		case opDispatch:
			if p.T == "READY" {
				var ready struct {
					SessionID string `json:"session_id"`
				}
				_ = json.Unmarshal(p.D, &ready)
				g.mu.Lock()
				g.sessionID = ready.SessionID
				g.mu.Unlock()
				logging.Info("discord gateway: authenticated (READY received)")
			}
		case opReconnect:
			return fmt.Errorf("server requested reconnect")
		case opInvalidSession:
			return fmt.Errorf("invalid session (bad token, or rate limited)")
		case opHeartbeatACK:
			// nothing to do
		}
	}
}

func (g *Gateway) heartbeatLoop(conn *websocket.Conn, interval time.Duration) {
	// Jitter the first beat as the real clients do.
	timer := time.NewTimer(time.Duration(rand.Float64() * float64(interval)))
	defer timer.Stop()

	for {
		select {
		case <-g.stopHeartbeat:
			return
		case <-timer.C:
			g.mu.Lock()
			seq := g.seq
			g.mu.Unlock()

			d, _ := json.Marshal(seq)
			hb := payload{Op: opHeartbeat, D: d}
			if err := conn.WriteJSON(hb); err != nil {
				return
			}
			timer.Reset(interval)
		}
	}
}

func (g *Gateway) identify(conn *websocket.Conn) error {
	data := identifyData{
		Token: g.token,
		Properties: identifyProperties{
			OS:      "linux",
			Browser: "jellyrpc",
			Device:  "jellyrpc",
		},
	}
	d, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return conn.WriteJSON(payload{Op: opIdentify, D: d})
}

// SetActivity pushes a Rich Presence update. Passing nil disconnects the
// gateway so it won't stay online while idle.
func (g *Gateway) SetActivity(activity *Activity) error {
	g.mu.Lock()
	g.lastActivity = activity
	conn := g.conn
	g.mu.Unlock()

	if activity == nil {
		if conn != nil {
			_ = g.sendPresence(conn, Activity{})
		}
		g.disconnect()
		return nil
	}
	if conn == nil {
		select {
		case g.reconnectCh <- struct{}{}:
		default:
		}
		return nil
	}
	return g.sendPresence(conn, *activity)
}

func (g *Gateway) disconnect() {
	g.mu.Lock()
	conn := g.conn
	g.conn = nil
	g.stopHeartbeat = nil
	g.closing = true
	g.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
}

func (g *Gateway) isClosing() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closing
}

func isExpectedDisconnectError(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

func (g *Gateway) sendPresence(conn *websocket.Conn, activity Activity) error {
	update := buildPresenceUpdate(&activity, "online")
	if activity.Name == "" {
		update.Status = "invisible"
	}

	d, err := json.Marshal(update)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return conn.WriteJSON(payload{Op: opPresenceUpdate, D: d})
}

func buildPresenceUpdate(activity *Activity, status string) presenceUpdate {
	activities := []Activity{}
	if activity != nil && activity.Name != "" {
		activities = append(activities, *activity)
	}
	return presenceUpdate{
		Since:      nil,
		Activities: activities,
		Status:     status,
		AFK:        false,
	}
}
