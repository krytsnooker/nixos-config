package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type PathGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// TargetParam1/TargetParam2 are generic, optional parameters with no fixed
// meaning in this service — a separate integration layer, built per target
// application, decides what each one means and does the actual push.
type PathEntry struct {
	ID                     string `json:"id"`
	ApplicationPathGroupID string `json:"applicationPathGroupId"`
	Name                   string `json:"name"`
	HostPath               string `json:"hostPath"`
	RelativePath           string `json:"relativePath"`
	TargetParam1           string `json:"targetParam1,omitempty"`
	TargetParam2           string `json:"targetParam2,omitempty"`
}

type pathGroupData struct {
	Groups  []PathGroup `json:"groups"`
	Entries []PathEntry `json:"entries"`
}

type GroupStore struct {
	mu   sync.RWMutex
	path string
	data pathGroupData
}

func NewGroupStore(path string) (*GroupStore, error) {
	gs := &GroupStore{path: path, data: pathGroupData{Groups: []PathGroup{}, Entries: []PathEntry{}}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return gs, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &gs.data); err != nil {
		return nil, err
	}
	return gs, nil
}

func (gs *GroupStore) save() error {
	b, err := json.MarshalIndent(gs.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := gs.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, gs.path)
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// findGroup looks up a group by ID. Caller must hold s.groups.mu (read or write).
func (s *Server) findGroup(id string) (PathGroup, bool) {
	for _, g := range s.groups.data.Groups {
		if g.ID == id {
			return g, true
		}
	}
	return PathGroup{}, false
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.groups.mu.RLock()
		defer s.groups.mu.RUnlock()
		json.NewEncoder(w).Encode(s.groups.data.Groups)

	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}
		g := PathGroup{ID: newID(), Name: req.Name, Description: req.Description}
		s.groups.mu.Lock()
		s.groups.data.Groups = append(s.groups.data.Groups, g)
		err := s.groups.save()
		s.groups.mu.Unlock()
		if err != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(g)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGroupByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")

	s.groups.mu.Lock()
	defer s.groups.mu.Unlock()
	groups := make([]PathGroup, 0, len(s.groups.data.Groups))
	for _, g := range s.groups.data.Groups {
		if g.ID != id {
			groups = append(groups, g)
		}
	}
	entries := make([]PathEntry, 0, len(s.groups.data.Entries))
	for _, e := range s.groups.data.Entries {
		if e.ApplicationPathGroupID != id {
			entries = append(entries, e)
		}
	}
	s.groups.data.Groups = groups
	s.groups.data.Entries = entries
	if err := s.groups.save(); err != nil {
		http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("id")
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		s.groups.mu.RLock()
		_, ok := s.findGroup(groupID)
		var entries []PathEntry
		for _, e := range s.groups.data.Entries {
			if e.ApplicationPathGroupID == groupID {
				entries = append(entries, e)
			}
		}
		s.groups.mu.RUnlock()
		if !ok {
			http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
			return
		}
		if entries == nil {
			entries = []PathEntry{}
		}
		json.NewEncoder(w).Encode(entries)

	case http.MethodPost:
		var req struct {
			Name         string `json:"name"`
			Path         string `json:"path"`
			TargetParam1 string `json:"targetParam1"`
			TargetParam2 string `json:"targetParam2"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Path == "" {
			http.Error(w, `{"error":"name and path are required"}`, http.StatusBadRequest)
			return
		}
		absPath, err := s.resolveBrowsePath(req.Path)
		if err != nil {
			http.Error(w, `{"error":"path must be under the media root"}`, http.StatusBadRequest)
			return
		}

		s.groups.mu.Lock()
		_, ok := s.findGroup(groupID)
		if !ok {
			s.groups.mu.Unlock()
			http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
			return
		}
		entry := PathEntry{
			ID:                     newID(),
			ApplicationPathGroupID: groupID,
			Name:                   req.Name,
			HostPath:               absPath,
			RelativePath:           filepath.Clean("/" + req.Path),
			TargetParam1:           req.TargetParam1,
			TargetParam2:           req.TargetParam2,
		}
		s.groups.data.Entries = append(s.groups.data.Entries, entry)
		saveErr := s.groups.save()
		s.groups.mu.Unlock()
		if saveErr != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(entry)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEntryByID(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("id")
	entryID := r.PathValue("entryId")
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPut:
		var req struct {
			Name         string `json:"name"`
			Path         string `json:"path"`
			TargetParam1 string `json:"targetParam1"`
			TargetParam2 string `json:"targetParam2"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}

		s.groups.mu.Lock()
		var updated *PathEntry
		for i := range s.groups.data.Entries {
			e := &s.groups.data.Entries[i]
			if e.ID != entryID || e.ApplicationPathGroupID != groupID {
				continue
			}
			if req.Name != "" {
				e.Name = req.Name
			}
			if req.TargetParam1 != "" {
				e.TargetParam1 = req.TargetParam1
			}
			if req.TargetParam2 != "" {
				e.TargetParam2 = req.TargetParam2
			}
			if req.Path != "" {
				abs, err := s.resolveBrowsePath(req.Path)
				if err != nil {
					s.groups.mu.Unlock()
					http.Error(w, `{"error":"path must be under the media root"}`, http.StatusBadRequest)
					return
				}
				e.HostPath = abs
				e.RelativePath = filepath.Clean("/" + req.Path)
			}
			updated = e
		}
		if updated == nil {
			s.groups.mu.Unlock()
			http.Error(w, `{"error":"entry not found"}`, http.StatusNotFound)
			return
		}
		entryCopy := *updated
		saveErr := s.groups.save()
		s.groups.mu.Unlock()
		if saveErr != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(entryCopy)

	case http.MethodDelete:
		s.groups.mu.Lock()
		entries := make([]PathEntry, 0, len(s.groups.data.Entries))
		for _, e := range s.groups.data.Entries {
			if e.ID != entryID || e.ApplicationPathGroupID != groupID {
				entries = append(entries, e)
			}
		}
		s.groups.data.Entries = entries
		err := s.groups.save()
		s.groups.mu.Unlock()
		if err != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
