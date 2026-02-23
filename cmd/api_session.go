package cmd

import (
	"encoding/json"
	"net/http"

	"github.com/Siddhant-K-code/distill/pkg/session"
)

// SessionAPI handles session-related HTTP endpoints.
type SessionAPI struct {
	store *session.SQLiteStore
}

// RegisterSessionRoutes adds session endpoints to the given mux.
func (s *SessionAPI) RegisterSessionRoutes(mux *http.ServeMux, mw func(string, http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/v1/session/create", mw("/v1/session/create", s.handleCreate))
	mux.HandleFunc("/v1/session/push", mw("/v1/session/push", s.handlePush))
	mux.HandleFunc("/v1/session/context", mw("/v1/session/context", s.handleContext))
	mux.HandleFunc("/v1/session/delete", mw("/v1/session/delete", s.handleDelete))
	mux.HandleFunc("/v1/session/get", mw("/v1/session/get", s.handleGet))
}

func (s *SessionAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req session.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	sess, err := s.store.Create(r.Context(), req)
	if err != nil {
		if err == session.ErrSessionExists {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sess)
}

func (s *SessionAPI) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req session.PushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	result, err := s.store.Push(r.Context(), req)
	if err != nil {
		if err == session.ErrSessionNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err == session.ErrOverBudget {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *SessionAPI) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req session.ContextRequest

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		req.SessionID = r.URL.Query().Get("session_id")
		req.Role = r.URL.Query().Get("role")
	}

	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	result, err := s.store.Context(r.Context(), req)
	if err != nil {
		if err == session.ErrSessionNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *SessionAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" && r.Method == http.MethodPost {
		var body struct {
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			sessionID = body.SessionID
		}
	}

	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	sess, err := s.store.Get(r.Context(), sessionID)
	if err != nil {
		if err == session.ErrSessionNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sess)
}

func (s *SessionAPI) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	result, err := s.store.Delete(r.Context(), req.SessionID)
	if err != nil {
		if err == session.ErrSessionNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
