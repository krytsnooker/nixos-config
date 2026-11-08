package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"codebreakers/internal/game"
)

type GameStore struct {
	mu    sync.RWMutex
	games map[string]*game.Game
}

func NewGameStore() *GameStore {
	return &GameStore{games: make(map[string]*game.Game)}
}

func NewID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *GameStore) Create(g *game.Game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games[g.ID] = g
}

func (s *GameStore) Get(id string) (*game.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return nil, errors.New("game not found")
	}
	return g, nil
}

func (s *GameStore) WithLock(id string, fn func(*game.Game) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.games[id]
	if !ok {
		return errors.New("game not found")
	}
	return fn(g)
}

func (s *GameStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.games, id)
}

func (s *GameStore) ForEach(fn func(*game.Game)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.games {
		fn(g)
	}
}
