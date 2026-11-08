package httpapi

import "net/http"

func NewRouter(h *Handler, webDir string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/games", h.ListGames)
	mux.HandleFunc("POST /api/games", h.CreateGame)
	mux.HandleFunc("POST /api/games/{id}/join", func(w http.ResponseWriter, r *http.Request) {
		h.JoinGame(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("POST /api/games/{id}/leave", func(w http.ResponseWriter, r *http.Request) {
		h.LeaveGame(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("DELETE /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		h.EndGame(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("POST /api/games/{id}/code", func(w http.ResponseWriter, r *http.Request) {
		h.SetCode(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("POST /api/games/{id}/guess", func(w http.ResponseWriter, r *http.Request) {
		h.Guess(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("GET /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		h.GetState(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("GET /ws/{id}", func(w http.ResponseWriter, r *http.Request) {
		h.ServeWS(w, r, r.PathValue("id"))
	})

	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	return mux
}
