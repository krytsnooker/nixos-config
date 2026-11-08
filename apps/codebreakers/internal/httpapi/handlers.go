package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/gorilla/websocket"

	"codebreakers/internal/game"
	"codebreakers/internal/storage"
)

type Handler struct {
	store *storage.GameStore
	hub   *Hub
}

func NewHandler(store *storage.GameStore, hub *Hub) *Handler {
	return &Handler{store: store, hub: hub}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type stateView struct {
	ID        string        `json:"id"`
	MakerID   string        `json:"makerId"`
	MakerName string        `json:"makerName"`
	Breakers  []game.Player `json:"breakers"`
	History   []game.Guess  `json:"history"`
	Status    game.Status   `json:"status"`
	TurnIndex int           `json:"turnIndex"`
	Code      []game.Peg    `json:"code,omitempty"`
}

// viewOf shapes the game state for one specific viewer: the codemaker
// always sees the code they set, everyone else only sees it once the game
// has finished.
func viewOf(g *game.Game, viewerID string) stateView {
	v := stateView{
		ID:        g.ID,
		MakerID:   g.MakerID,
		MakerName: g.MakerName,
		Breakers:  g.Breakers,
		History:   g.History,
		Status:    g.Status,
		TurnIndex: g.TurnIndex,
	}
	finished := g.Status == game.StatusBreakersWon || g.Status == game.StatusMakerWon
	if finished || viewerID == g.MakerID {
		v.Code = g.RevealCode()
	}
	return v
}

// viewsByPlayer builds the tailored view for every current participant
// (maker + each breaker), to be broadcast while the game is still locked.
// spectatorView is the fallback for any viewer that is neither (hidden code).
func viewsByPlayer(g *game.Game) (views map[string]any, spectatorView stateView) {
	views = make(map[string]any, len(g.Breakers)+1)
	views[g.MakerID] = viewOf(g, g.MakerID)
	for _, b := range g.Breakers {
		views[b.ID] = viewOf(g, b.ID)
	}
	return views, viewOf(g, "")
}

type gameSummary struct {
	ID          string      `json:"id"`
	MakerName   string      `json:"makerName"`
	Status      game.Status `json:"status"`
	NumBreakers int         `json:"numBreakers"`
	createdAt   int64
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (h *Handler) ListGames(w http.ResponseWriter, r *http.Request) {
	var summaries []gameSummary
	h.store.ForEach(func(g *game.Game) {
		if g.Status == game.StatusBreakersWon || g.Status == game.StatusMakerWon {
			return
		}
		summaries = append(summaries, gameSummary{
			ID:          g.ID,
			MakerName:   g.MakerName,
			Status:      g.Status,
			NumBreakers: len(g.Breakers),
			createdAt:   g.CreatedAt.UnixNano(),
		})
	})
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].createdAt > summaries[j].createdAt
	})
	writeJSON(w, http.StatusOK, summaries)
}

func (h *Handler) CreateGame(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	gameID, err := storage.NewID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	makerID, err := storage.NewID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	g := game.NewGame(gameID, makerID, req.Name)
	h.store.Create(g)

	writeJSON(w, http.StatusCreated, map[string]string{
		"gameId":   gameID,
		"playerId": makerID,
	})
}

func (h *Handler) JoinGame(w http.ResponseWriter, r *http.Request, gameID string) {
	var req struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	playerID, err := storage.NewID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	var views map[string]any
	var spectatorView stateView
	err = h.store.WithLock(gameID, func(g *game.Game) error {
		if err := g.AddBreaker(playerID, req.Name); err != nil {
			return err
		}
		views, spectatorView = viewsByPlayer(g)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	h.hub.BroadcastPerPlayer(gameID, views, spectatorView)
	writeJSON(w, http.StatusOK, map[string]string{"playerId": playerID})
}

func (h *Handler) LeaveGame(w http.ResponseWriter, r *http.Request, gameID string) {
	var req struct{ PlayerID string `json:"playerId"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if g, err := h.store.Get(gameID); err == nil && g.MakerID == req.PlayerID {
		h.store.Delete(gameID)
		h.hub.Terminate(gameID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "terminated"})
		return
	}

	var views map[string]any
	var spectatorView stateView
	err := h.store.WithLock(gameID, func(g *game.Game) error {
		if err := g.RemoveBreaker(req.PlayerID); err != nil {
			return err
		}
		views, spectatorView = viewsByPlayer(g)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	h.hub.BroadcastPerPlayer(gameID, views, spectatorView)
	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

func (h *Handler) EndGame(w http.ResponseWriter, r *http.Request, gameID string) {
	if _, err := h.store.Get(gameID); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	h.store.Delete(gameID)
	h.hub.Terminate(gameID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "terminated"})
}

func (h *Handler) SetCode(w http.ResponseWriter, r *http.Request, gameID string) {
	var req struct {
		PlayerID string     `json:"playerId"`
		Pegs     []game.Peg `json:"pegs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	var views map[string]any
	var spectatorView stateView
	var myView stateView
	err := h.store.WithLock(gameID, func(g *game.Game) error {
		if err := g.SetCode(req.PlayerID, req.Pegs); err != nil {
			return err
		}
		views, spectatorView = viewsByPlayer(g)
		myView = views[req.PlayerID].(stateView)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	h.hub.BroadcastPerPlayer(gameID, views, spectatorView)
	writeJSON(w, http.StatusOK, myView)
}

func (h *Handler) Guess(w http.ResponseWriter, r *http.Request, gameID string) {
	var req struct {
		PlayerID string     `json:"playerId"`
		Pegs     []game.Peg `json:"pegs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	var views map[string]any
	var spectatorView stateView
	var myView stateView
	err := h.store.WithLock(gameID, func(g *game.Game) error {
		if _, err := g.Guess(req.PlayerID, req.Pegs); err != nil {
			return err
		}
		views, spectatorView = viewsByPlayer(g)
		myView = views[req.PlayerID].(stateView)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	h.hub.BroadcastPerPlayer(gameID, views, spectatorView)
	writeJSON(w, http.StatusOK, myView)
}

func (h *Handler) GetState(w http.ResponseWriter, r *http.Request, gameID string) {
	playerID := r.URL.Query().Get("playerId")
	g, err := h.store.Get(gameID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(g, playerID))
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request, gameID string) {
	playerID := r.URL.Query().Get("playerId")

	var initial stateView
	err := h.store.WithLock(gameID, func(g *game.Game) error {
		initial = viewOf(g, playerID)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	h.hub.Subscribe(gameID, conn, playerID)
	conn.WriteJSON(initial)

	go func() {
		defer func() {
			h.hub.Unsubscribe(gameID, conn)
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}
