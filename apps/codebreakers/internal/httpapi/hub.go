package httpapi

import (
	"context"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*websocket.Conn]string
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[*websocket.Conn]string)}
}

func (h *Hub) Subscribe(gameID string, conn *websocket.Conn, playerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[gameID] == nil {
		h.subs[gameID] = make(map[*websocket.Conn]string)
	}
	h.subs[gameID][conn] = playerID
}

func (h *Hub) Unsubscribe(gameID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[gameID], conn)
}

// BroadcastPerPlayer sends viewsByPlayer[playerID] to each subscriber of
// gameID, falling back to defaultView for any subscriber whose playerID
// isn't in the map. viewsByPlayer must be fully built by the caller
// beforehand (e.g. while still holding a lock over shared game state) since
// this method does no further reads of game state.
func (h *Hub) BroadcastPerPlayer(gameID string, viewsByPlayer map[string]any, defaultView any) {
	h.mu.RLock()
	conns := make(map[*websocket.Conn]string, len(h.subs[gameID]))
	for c, playerID := range h.subs[gameID] {
		conns[c] = playerID
	}
	h.mu.RUnlock()

	for c, playerID := range conns {
		view, ok := viewsByPlayer[playerID]
		if !ok {
			view = defaultView
		}
		if err := wsjson.Write(context.Background(), c, view); err != nil {
			h.Unsubscribe(gameID, c)
			c.CloseNow()
		}
	}
}

// Terminate notifies every current subscriber of gameID that the game is
// gone, then closes their connections and drops the subscriber list.
func (h *Hub) Terminate(gameID string) {
	h.mu.Lock()
	conns := h.subs[gameID]
	delete(h.subs, gameID)
	h.mu.Unlock()

	for c := range conns {
		wsjson.Write(context.Background(), c, map[string]bool{"terminated": true})
		c.Close(websocket.StatusNormalClosure, "game terminated")
	}
}
